// IoT Smart Boxing Analytics — Go Server
//
// Responsibilities:
//   - UDP :4210  → receive raw MPU6050 samples from ESP32 at ~100Hz
//   - Perform all analysis: magnitude, punch detection, debounce, stats
//   - WebSocket :8080/ws → broadcast SessionState to React dashboard
//   - HTTP :8080         → serve embedded React build + REST session API
//
// No external dependencies — stdlib only.

package main

import (
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ─── Embed React build ────────────────────────────────────────────────────────
// Run `npm run build` in dashboard/ and copy dist/ contents → server/static/
// before using the embedded static server.
// During development use `npm run dev` (Vite proxies /ws and /api to :8080).

//go:embed static
var staticFiles embed.FS

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	udpPort        = ":4210"
	httpPort       = ":8080"
	punchThreshold = 35.0 // m/s² — magnitude above this = punch candidate
	debounceMS     = 300  // ms   — minimum gap between two punch events
	rollingBufSize = 500  // samples @ 100Hz = 5 seconds of history
	maxRecentPunch = 50   // punches kept in chart history
	wsGUID         = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// RawPacket is the JSON sent by the ESP32 for every sensor sample.
type RawPacket struct {
	Type string  `json:"type"` // "data" | "discover" | "session_reset_ack"
	AX   float64 `json:"ax"`
	AY   float64 `json:"ay"`
	AZ   float64 `json:"az"`
	TS   int64   `json:"ts"` // millis() on ESP32
}

// PunchEvent records a detected punch.
type PunchEvent struct {
	Force float64 `json:"force"` // m/s²
	TS    int64   `json:"ts"`    // ESP32 millis
	Count int     `json:"count"`
}

// SessionState is the full state broadcast to all WebSocket clients.
type SessionState struct {
	Active        bool         `json:"active"`
	PunchCount    int          `json:"punch_count"`
	MaxForce      float64      `json:"max_force"`
	AvgForce      float64      `json:"avg_force"`
	PunchesPerMin float64      `json:"ppm"`
	RecentPunches []PunchEvent `json:"recent_punches"` // last 50 for chart
	ElapsedSec    float64      `json:"elapsed_sec"`
}

// RollingBuffer is a fixed-size circular buffer of float64 magnitudes.
type RollingBuffer struct {
	data  [rollingBufSize]float64
	head  int
	count int
}

func (r *RollingBuffer) Push(v float64) {
	r.data[r.head] = v
	r.head = (r.head + 1) % rollingBufSize
	if r.count < rollingBufSize {
		r.count++
	}
}

// ─── WebSocket Hub ────────────────────────────────────────────────────────────
// Minimal WebSocket hub using stdlib only (RFC 6455 framing).

type wsClient struct {
	conn net.Conn
	send chan []byte
}

// Hub manages connected WebSocket clients and broadcasts to all of them.
type Hub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
}

func newHub() *Hub {
	return &Hub{clients: make(map[*wsClient]struct{})}
}

func (h *Hub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *wsClient) {
	h.mu.Lock()
	_, exists := h.clients[c]
	if exists {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// Broadcast sends a WebSocket text frame to all connected clients.
func (h *Hub) Broadcast(payload []byte) {
	frame := makeWsTextFrame(payload)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- frame:
		default:
			// Slow client — drop frame rather than block
		}
	}
}

// makeWsTextFrame builds a minimal unmasked WebSocket text frame (RFC 6455).
func makeWsTextFrame(payload []byte) []byte {
	length := len(payload)
	var header []byte
	switch {
	case length < 126:
		header = []byte{0x81, byte(length)}
	case length < 65536:
		header = []byte{0x81, 126, byte(length >> 8), byte(length)}
	default:
		header = []byte{0x81, 127,
			0, 0, 0, 0,
			byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length),
		}
	}
	return append(header, payload...)
}

// ─── Analytics Engine ─────────────────────────────────────────────────────────

// Analyzer receives raw sensor packets and computes all boxing metrics.
type Analyzer struct {
	mu          sync.Mutex
	session     SessionState
	rollBuf     RollingBuffer
	lastPunchTS int64 // ESP32 millis at last detected punch
	forceSum    float64
	startedAt   time.Time
	esp32Addr   *net.UDPAddr
	udpConn     *net.UDPConn
	hub         *Hub
}

func newAnalyzer(udpConn *net.UDPConn, hub *Hub) *Analyzer {
	return &Analyzer{
		udpConn: udpConn,
		hub:     hub,
		session: SessionState{RecentPunches: []PunchEvent{}},
	}
}

// ProcessSample handles one raw sensor reading from the ESP32.
func (a *Analyzer) ProcessSample(pkt RawPacket, src *net.UDPAddr) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Lock onto ESP32 address for sending commands back
	if a.esp32Addr == nil {
		a.esp32Addr = src
		log.Printf("ESP32 streaming from %s", src)
	}

	if !a.session.Active {
		return // Ignore data until session started from dashboard
	}

	// 1. Compute vector magnitude (m/s²)
	mag := math.Sqrt(pkt.AX*pkt.AX + pkt.AY*pkt.AY + pkt.AZ*pkt.AZ)

	// 2. Push to 5-second rolling buffer
	a.rollBuf.Push(mag)

	// 3. Punch detection: threshold + debounce
	timeSinceLast := pkt.TS - a.lastPunchTS
	if mag > punchThreshold && timeSinceLast > debounceMS {
		a.session.PunchCount++
		a.lastPunchTS = pkt.TS

		if mag > a.session.MaxForce {
			a.session.MaxForce = mag
		}

		a.forceSum += mag
		a.session.AvgForce = a.forceSum / float64(a.session.PunchCount)

		elapsed := time.Since(a.startedAt).Minutes()
		if elapsed > 0 {
			a.session.PunchesPerMin = float64(a.session.PunchCount) / elapsed
		}

		event := PunchEvent{Force: math.Round(mag*100) / 100, TS: pkt.TS, Count: a.session.PunchCount}
		a.session.RecentPunches = append(a.session.RecentPunches, event)
		if len(a.session.RecentPunches) > maxRecentPunch {
			a.session.RecentPunches = a.session.RecentPunches[1:]
		}

		log.Printf("PUNCH #%d  force=%.1f m/s²  avg=%.1f  max=%.1f  ppm=%.1f",
			a.session.PunchCount, mag,
			a.session.AvgForce, a.session.MaxForce,
			a.session.PunchesPerMin,
		)

		a.broadcastLocked()
	}
}

// BroadcastTick sends elapsed time updates every second (called by ticker goroutine).
func (a *Analyzer) BroadcastTick() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session.Active {
		a.session.ElapsedSec = time.Since(a.startedAt).Seconds()
		a.broadcastLocked()
	}
}

// broadcastLocked serialises session state and sends to all WS clients.
// Must be called with a.mu held.
func (a *Analyzer) broadcastLocked() {
	data, err := json.Marshal(a.session)
	if err != nil {
		log.Printf("json.Marshal: %v", err)
		return
	}
	a.hub.Broadcast(data)
}

// StartSession activates a new session and notifies the ESP32.
func (a *Analyzer) StartSession() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.session = SessionState{
		Active:        true,
		RecentPunches: []PunchEvent{},
	}
	a.rollBuf     = RollingBuffer{}
	a.lastPunchTS = 0
	a.forceSum    = 0
	a.startedAt   = time.Now()
	log.Println("Session started.")
	a.sendCommandLocked("session_start")
	a.broadcastLocked()
}

// ResetSession clears all stats and deactivates the session.
func (a *Analyzer) ResetSession() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.session = SessionState{
		Active:        false,
		RecentPunches: []PunchEvent{},
	}
	a.rollBuf     = RollingBuffer{}
	a.lastPunchTS = 0
	a.forceSum    = 0
	log.Println("Session reset.")
	a.sendCommandLocked("session_reset")
	a.broadcastLocked()
}

// sendCommandLocked sends a JSON command to the ESP32 via UDP.
// Must be called with a.mu held.
func (a *Analyzer) sendCommandLocked(cmdType string) {
	if a.esp32Addr == nil {
		log.Println("Cannot send command: ESP32 not yet connected.")
		return
	}
	msg := fmt.Sprintf(`{"type":%q}`, cmdType)
	if _, err := a.udpConn.WriteToUDP([]byte(msg), a.esp32Addr); err != nil {
		log.Printf("UDP send error: %v", err)
	}
}

// ─── UDP Listener ─────────────────────────────────────────────────────────────

func runUDPListener(conn *net.UDPConn, analyzer *Analyzer) {
	buf := make([]byte, 256)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("UDP read: %v", err)
			continue
		}

		var pkt RawPacket
		if err := json.Unmarshal(buf[:n], &pkt); err != nil {
			log.Printf("UDP parse: %v (raw: %s)", err, buf[:n])
			continue
		}

		switch pkt.Type {
		case "discover":
			ack := []byte(`{"type":"ack"}`)
			if _, err := conn.WriteToUDP(ack, src); err != nil {
				log.Printf("ACK send: %v", err)
			} else {
				log.Printf("Discovery ACK → %s", src)
			}
		case "data":
			analyzer.ProcessSample(pkt, src)
		case "session_reset_ack":
			log.Println("ESP32 acknowledged session reset.")
		default:
			log.Printf("Unknown packet type: %q", pkt.Type)
		}
	}
}

// ─── WebSocket Handshake (stdlib, RFC 6455) ───────────────────────────────────

func wsAcceptKey(key string) string {
	h := sha1.New()
	h.Write([]byte(strings.TrimSpace(key) + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func upgradeToWS(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("hijacking not supported")
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAcceptKey(key) + "\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// ─── HTTP Handlers ────────────────────────────────────────────────────────────

func wsHandler(hub *Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgradeToWS(w, r)
		if err != nil {
			log.Printf("WS upgrade: %v", err)
			http.Error(w, "WS upgrade failed", http.StatusBadRequest)
			return
		}

		client := &wsClient{conn: conn, send: make(chan []byte, 64)}
		hub.register(client)
		log.Printf("WS client connected: %s", conn.RemoteAddr())

		// Write pump
		go func() {
			defer func() {
				conn.Close()
				log.Printf("WS client disconnected: %s", conn.RemoteAddr())
			}()
			for frame := range client.send {
				if _, err := conn.Write(frame); err != nil {
					return
				}
			}
		}()

		// Read pump — consume frames to detect client disconnect
		rbuf := make([]byte, 512)
		for {
			if _, err := conn.Read(rbuf); err != nil {
				break
			}
		}
		hub.unregister(client)
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// UDP server — plain listen, ESP32 now pings us directly (no broadcast)
	udpAddr, err := net.ResolveUDPAddr("udp4", udpPort)
	if err != nil {
		log.Fatalf("UDP resolve: %v", err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		log.Fatalf("UDP listen: %v", err)
	}
	defer udpConn.Close()
	log.Printf("UDP listening on %s", udpPort)

	hub      := newHub()
	analyzer := newAnalyzer(udpConn, hub)

	// UDP listener goroutine
	go runUDPListener(udpConn, analyzer)

	// Ticker: broadcast elapsed time every second
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			analyzer.BroadcastTick()
		}
	}()

	// HTTP mux
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler(hub))

	mux.HandleFunc("/api/session/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.StartSession()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/api/session/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.ResetSession()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	// Embedded React build (production mode)
	// In development, Vite runs on :5173 and proxies /ws + /api to :8080
	stripped, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(stripped)))

	log.Printf("HTTP/WS server on %s", httpPort)
	if err := http.ListenAndServe(httpPort, mux); err != nil {
		log.Fatalf("HTTP listen: %v", err)
	}
}
