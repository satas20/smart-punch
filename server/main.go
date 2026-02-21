// FighterLink Boxing Analytics Server
//
// BLE Central that connects to FighterLink_L and FighterLink_R gloves,
// processes sensor data for punch detection/classification, and broadcasts
// analytics to WebSocket clients.
//
// Responsibilities:
//   - BLE Central: Connect to dual gloves, receive 100Hz sensor data
//   - Analytics: Punch detection, classification (straight/hook/uppercut)
//   - WebSocket: Broadcast SessionState to React dashboard
//   - HTTP: Serve embedded React build + REST session API

package main

import (
	"crypto/sha1"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"boxing-analytics/analytics"
	"boxing-analytics/ble"
)

// ─── Embed React build ────────────────────────────────────────────────────────

//go:embed static
var staticFiles embed.FS

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	httpPort = ":8080"
	wsGUID   = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

// ─── WebSocket Hub ────────────────────────────────────────────────────────────

type wsClient struct {
	conn net.Conn
	send chan []byte
}

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

func (h *Hub) Broadcast(payload []byte) {
	frame := makeWsTextFrame(payload)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.send <- frame:
		default:
			// Slow client — drop frame
		}
	}
}

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

// ─── WebSocket Handshake ──────────────────────────────────────────────────────

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

func wsHandler(hub *Hub, analyzer *analytics.Analyzer) http.HandlerFunc {
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

		// Send current state immediately
		state := analyzer.GetState()
		if data, err := json.Marshal(state); err == nil {
			client.send <- makeWsTextFrame(data)
		}

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

		// Read pump — consume frames to detect disconnect
		rbuf := make([]byte, 512)
		for {
			if _, err := conn.Read(rbuf); err != nil {
				break
			}
		}
		hub.unregister(client)
	}
}

func sessionStartHandler(analyzer *analytics.Analyzer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.StartSession()
		log.Println("Session started")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func sessionResetHandler(analyzer *analytics.Analyzer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.ResetSession()
		log.Println("Session reset")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func sessionPauseHandler(analyzer *analytics.Analyzer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.PauseSession()
		log.Println("Session paused")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func sessionResumeHandler(analyzer *analytics.Analyzer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		analyzer.ResumeSession()
		log.Println("Session resumed")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func sessionStopHandler(analyzer *analytics.Analyzer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		// Stop keeps the stats but marks session as inactive
		analyzer.ResetSession() // For now, same as reset - stats are kept in frontend
		log.Println("Session stopped")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func statusHandler(central *ble.Central) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := map[string]interface{}{
			"left_connected":  central.IsConnected(ble.LeftHand),
			"right_connected": central.IsConnected(ble.RightHand),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	// Suppress go-bluetooth library warnings (MapToStruct: invalid field detected)
	// These are harmless warnings from the library not having all BlueZ properties mapped
	logrus.SetLevel(logrus.ErrorLevel)

	log.Println("========================================")
	log.Println("FighterLink Boxing Analytics Server")
	log.Println("========================================")

	// Check for debug mode
	debugBLE := os.Getenv("DEBUG_BLE") == "1"
	if debugBLE {
		log.Println("BLE debug mode enabled")
	}

	// Create components
	hub := newHub()
	analyzer := analytics.NewAnalyzer()
	central := ble.NewCentral()

	// Set up state broadcast to WebSocket clients
	analyzer.SetStateHandler(func(state *analytics.SessionState) {
		data, err := json.Marshal(state)
		if err != nil {
			log.Printf("JSON marshal error: %v", err)
			return
		}
		hub.Broadcast(data)
	})

	// Set up packet handler from BLE
	central.SetPacketHandler(func(hand ble.Hand, packet *ble.SensorPacket) {
		analyzer.ProcessPacket(hand, packet)

		if debugBLE {
			log.Printf("BLE [%s]: %s", hand, packet)
		}
	})

	// Initialize BLE adapter
	if err := central.Enable(); err != nil {
		log.Fatalf("Failed to enable BLE: %v", err)
	}

	// Create scanner for auto-discovery
	scanner := ble.NewScanner(central, ble.DefaultScanConfig())

	// Start scanning for gloves
	scanner.Start()
	log.Println("Scanning for FighterLink_L and FighterLink_R...")

	// Ticker: broadcast elapsed time and log sensor data every second
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		// Track previous calibration state for logging state changes
		var leftWasCalibrated, rightWasCalibrated bool

		for range ticker.C {
			analyzer.BroadcastTick()

			// Update connection status in analyzer
			analyzer.SetConnected(ble.LeftHand, central.IsConnected(ble.LeftHand))
			analyzer.SetConnected(ble.RightHand, central.IsConnected(ble.RightHand))

			// Get current state for logging
			state := analyzer.GetState()

			// Log left hand sensor data if connected
			if state.Left.Connected {
				calStr := "UNCALIBRATED"
				if state.Left.Calibrated {
					calStr = "calibrated"
				}

				// Log calibration state change
				if state.Left.Calibrated && !leftWasCalibrated {
					log.Println("Sensor: FighterLink_L calibration complete!")
				}
				leftWasCalibrated = state.Left.Calibrated

				log.Printf("Sensor [L] [%s]: Accel(%.2f, %.2f, %.2f) m/s² | Gyro(%.1f, %.1f, %.1f) °/s | Bat=%d%%",
					calStr,
					state.Left.CurrentAccel[0], state.Left.CurrentAccel[1], state.Left.CurrentAccel[2],
					state.Left.CurrentGyro[0], state.Left.CurrentGyro[1], state.Left.CurrentGyro[2],
					state.Left.Battery)
			}

			// Log right hand sensor data if connected
			if state.Right.Connected {
				calStr := "UNCALIBRATED"
				if state.Right.Calibrated {
					calStr = "calibrated"
				}

				// Log calibration state change
				if state.Right.Calibrated && !rightWasCalibrated {
					log.Println("Sensor: FighterLink_R calibration complete!")
				}
				rightWasCalibrated = state.Right.Calibrated

				log.Printf("Sensor [R] [%s]: Accel(%.2f, %.2f, %.2f) m/s² | Gyro(%.1f, %.1f, %.1f) °/s | Bat=%d%%",
					calStr,
					state.Right.CurrentAccel[0], state.Right.CurrentAccel[1], state.Right.CurrentAccel[2],
					state.Right.CurrentGyro[0], state.Right.CurrentGyro[1], state.Right.CurrentGyro[2],
					state.Right.Battery)
			}
		}
	}()

	// HTTP server setup
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", wsHandler(hub, analyzer))
	mux.HandleFunc("/api/session/start", sessionStartHandler(analyzer))
	mux.HandleFunc("/api/session/reset", sessionResetHandler(analyzer))
	mux.HandleFunc("/api/session/pause", sessionPauseHandler(analyzer))
	mux.HandleFunc("/api/session/resume", sessionResumeHandler(analyzer))
	mux.HandleFunc("/api/session/stop", sessionStopHandler(analyzer))
	mux.HandleFunc("/api/status", statusHandler(central))

	// Embedded React build
	stripped, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(stripped)))

	// Get port from environment or use default
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = httpPort
	}

	log.Printf("HTTP/WS server on %s", port)
	log.Println("Dashboard: http://localhost" + port)
	log.Println("")
	log.Println("Waiting for glove connections...")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("HTTP listen: %v", err)
	}
}
