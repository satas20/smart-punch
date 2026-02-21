// Package analytics provides punch detection and classification for FighterLink.
package analytics

import (
	"math"
	"sync"
	"time"

	"boxing-analytics/ble"
)

// ─── Constants ───────────────────────────────────────────────────────────────

const (
	// Punch detection thresholds
	punchThreshold = 35.0 // m/s² - minimum acceleration for punch detection
	debounceMS     = 300  // milliseconds between valid punches

	// Punch classification thresholds (gyroscope-based)
	hookGyroZThresh     = 200.0 // °/s - Z-axis rotation for hook detection
	uppercutGyroXThresh = 150.0 // °/s - X-axis rotation for uppercut detection
	straightGyroMax     = 150.0 // °/s - max rotation for straight punch

	// Stats tracking
	maxRecentPunches = 50  // punches kept in history
	rollingBufSize   = 500 // 5 seconds at 100Hz
)

// ─── Types ───────────────────────────────────────────────────────────────────

// PunchType represents the classification of a punch.
type PunchType string

const (
	PunchStraight PunchType = "straight"
	PunchHook     PunchType = "hook"
	PunchUppercut PunchType = "uppercut"
	PunchUnknown  PunchType = "unknown"
)

// PunchEvent represents a detected punch.
type PunchEvent struct {
	Hand      string    `json:"hand"`
	Type      PunchType `json:"type"`
	Force     float64   `json:"force"`      // m/s²
	RotationZ float64   `json:"rotation_z"` // peak °/s
	Timestamp int64     `json:"ts"`         // device timestamp
	Count     int       `json:"count"`      // punch number in session
}

// HandState holds analytics for one hand.
type HandState struct {
	Connected      bool           `json:"connected"`
	Calibrated     bool           `json:"calibrated"`
	Battery        uint8          `json:"battery"`
	PacketLoss     float64        `json:"packet_loss"`
	PunchCount     int            `json:"punch_count"`
	PunchBreakdown map[string]int `json:"punch_breakdown"`
	MaxForce       float64        `json:"max_force"`
	AvgForce       float64        `json:"avg_force"`
	PunchesPerMin  float64        `json:"ppm"`
	RecentPunches  []PunchEvent   `json:"recent_punches"`
	// Current sensor values (for logging/debugging)
	CurrentAccel  [3]float64 `json:"current_accel"` // X, Y, Z in m/s²
	CurrentGyro   [3]float64 `json:"current_gyro"`  // X, Y, Z in °/s
	forceSum      float64    // internal: sum of all punch forces
	lastPunchTS   int64      // internal: last punch timestamp (device)
	lastPunchTime time.Time  // internal: last punch time (local)
}

// CombinedStats holds aggregated stats from both hands.
type CombinedStats struct {
	TotalPunches   int     `json:"total_punches"`
	AvgForce       float64 `json:"avg_force"`
	MaxForce       float64 `json:"max_force"`
	PunchesPerMin  float64 `json:"ppm"`
	PunchesPerSec  float64 `json:"pps"`             // Real-time punch rate
	IntensityScore int     `json:"intensity_score"` // Gamified score: (punches * avgForce) / minutes
}

// SessionState is the full state broadcast to WebSocket clients.
type SessionState struct {
	Active     bool          `json:"active"`
	ElapsedSec float64       `json:"elapsed_sec"`
	Left       *HandState    `json:"left"`
	Right      *HandState    `json:"right"`
	Combined   CombinedStats `json:"combined"`
	Paused     bool          `json:"paused"` // true if a glove disconnected
}

// StateHandler is called when session state changes.
type StateHandler func(state *SessionState)

// ─── Analyzer ────────────────────────────────────────────────────────────────

// Analyzer processes sensor data and detects punches for both hands.
type Analyzer struct {
	mu        sync.RWMutex
	left      *HandState
	right     *HandState
	active    bool
	paused    bool
	startedAt time.Time
	onState   StateHandler
}

// NewAnalyzer creates a new Analyzer instance.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		left:  newHandState(),
		right: newHandState(),
	}
}

// newHandState creates an initialized HandState.
func newHandState() *HandState {
	return &HandState{
		PunchBreakdown: make(map[string]int),
		RecentPunches:  make([]PunchEvent, 0, maxRecentPunches),
	}
}

// SetStateHandler sets the callback for state changes.
func (a *Analyzer) SetStateHandler(handler StateHandler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onState = handler
}

// StartSession begins a new training session.
func (a *Analyzer) StartSession() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.left = newHandState()
	a.right = newHandState()
	a.active = true
	a.paused = false
	a.startedAt = time.Now()

	a.broadcastLocked()
}

// ResetSession clears all stats and stops the session.
func (a *Analyzer) ResetSession() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.left = newHandState()
	a.right = newHandState()
	a.active = false
	a.paused = false

	a.broadcastLocked()
}

// PauseSession pauses the session (e.g., when a glove disconnects).
func (a *Analyzer) PauseSession() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active {
		a.paused = true
		a.broadcastLocked()
	}
}

// ResumeSession resumes a paused session.
func (a *Analyzer) ResumeSession() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active && a.paused {
		a.paused = false
		a.broadcastLocked()
	}
}

// IsActive returns whether a session is currently active.
func (a *Analyzer) IsActive() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.active
}

// SetConnected updates the connection state for a hand.
func (a *Analyzer) SetConnected(hand ble.Hand, connected bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	state := a.left
	if hand == ble.RightHand {
		state = a.right
	}

	wasConnected := state.Connected
	state.Connected = connected

	// Pause only on a connected→disconnected transition during an active session,
	// not simply because a glove was never connected (single-glove mode).
	if a.active && wasConnected && !connected {
		a.paused = true
	}
	// Resume automatically when the dropped glove comes back.
	if a.active && a.paused && connected {
		a.paused = false
	}

	a.broadcastLocked()
}

// ProcessPacket handles an incoming sensor packet.
func (a *Analyzer) ProcessPacket(hand ble.Hand, packet *ble.SensorPacket) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Select the correct hand state
	state := a.left
	handName := "left"
	if hand == ble.RightHand {
		state = a.right
		handName = "right"
	}

	// Update battery and calibration status
	state.Battery = packet.Battery
	wasCalibrated := state.Calibrated
	state.Calibrated = packet.IsCalibrated()

	// Log calibration state change
	if !wasCalibrated && state.Calibrated {
		// Just became calibrated
		_ = handName // Will be used in logging from main.go
	}

	// Get acceleration and gyroscope values
	ax, ay, az := packet.AccelMS2()
	gx, gy, gz := packet.GyroDPS()

	// Store current sensor values (for logging/dashboard)
	state.CurrentAccel = [3]float64{ax, ay, az}
	state.CurrentGyro = [3]float64{gx, gy, gz}

	// Skip punch analysis if session not active or paused
	if !a.active || a.paused {
		return
	}

	// Calculate acceleration magnitude
	mag := math.Sqrt(ax*ax + ay*ay + az*az)

	// Punch detection: threshold + debounce
	timeSinceLast := int64(packet.Timestamp) - state.lastPunchTS
	if mag > punchThreshold && timeSinceLast > debounceMS {
		// Classify punch type based on gyroscope data
		punchType := classifyPunch(ax, ay, az, gx, gy, gz)

		// Update stats
		state.PunchCount++
		state.lastPunchTS = int64(packet.Timestamp)
		state.lastPunchTime = time.Now()

		if mag > state.MaxForce {
			state.MaxForce = mag
		}

		state.forceSum += mag
		state.AvgForce = state.forceSum / float64(state.PunchCount)

		// Calculate punches per minute
		elapsed := time.Since(a.startedAt).Minutes()
		if elapsed > 0 {
			state.PunchesPerMin = float64(state.PunchCount) / elapsed
		}

		// Update punch breakdown
		state.PunchBreakdown[string(punchType)]++

		// Create punch event
		event := PunchEvent{
			Hand:      handName,
			Type:      punchType,
			Force:     math.Round(mag*100) / 100,
			RotationZ: math.Abs(gz),
			Timestamp: int64(packet.Timestamp),
			Count:     state.PunchCount,
		}

		// Add to recent punches (limited buffer)
		state.RecentPunches = append(state.RecentPunches, event)
		if len(state.RecentPunches) > maxRecentPunches {
			state.RecentPunches = state.RecentPunches[1:]
		}

		// Broadcast state update
		a.broadcastLocked()
	}
}

// classifyPunch determines the punch type based on motion data.
func classifyPunch(ax, ay, az, gx, gy, gz float64) PunchType {
	// Get absolute values of gyroscope readings
	absGX := math.Abs(gx)
	absGY := math.Abs(gy)
	absGZ := math.Abs(gz)

	// Hook: High rotation around Z-axis (horizontal spin)
	if absGZ > hookGyroZThresh {
		return PunchHook
	}

	// Uppercut: High rotation around X-axis (vertical arc)
	if absGX > uppercutGyroXThresh {
		return PunchUppercut
	}

	// Straight: Low overall rotation
	maxRotation := math.Max(absGX, math.Max(absGY, absGZ))
	if maxRotation < straightGyroMax {
		return PunchStraight
	}

	// Unable to classify
	return PunchUnknown
}

// BroadcastTick sends periodic state updates (elapsed time).
func (a *Analyzer) BroadcastTick() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.active {
		a.broadcastLocked()
	}
}

// GetState returns the current session state.
func (a *Analyzer) GetState() *SessionState {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.buildStateLocked()
}

// buildStateLocked creates a SessionState snapshot.
// Must be called with a.mu held (read or write).
func (a *Analyzer) buildStateLocked() *SessionState {
	var elapsed float64
	if a.active {
		elapsed = time.Since(a.startedAt).Seconds()
	}

	// Build combined stats
	combined := CombinedStats{
		TotalPunches: a.left.PunchCount + a.right.PunchCount,
	}

	if combined.TotalPunches > 0 {
		totalForce := a.left.forceSum + a.right.forceSum
		combined.AvgForce = totalForce / float64(combined.TotalPunches)
		combined.MaxForce = math.Max(a.left.MaxForce, a.right.MaxForce)

		if elapsed > 0 {
			elapsedMin := elapsed / 60
			combined.PunchesPerMin = float64(combined.TotalPunches) / elapsedMin
			combined.PunchesPerSec = float64(combined.TotalPunches) / elapsed

			// Intensity Score: (punches * avgForce) / minutes
			// This creates a gamified "effort score" that rewards both volume and power
			if elapsedMin > 0.1 {
				combined.IntensityScore = int((float64(combined.TotalPunches) * combined.AvgForce) / elapsedMin)
			}
		}
	}

	return &SessionState{
		Active:     a.active,
		ElapsedSec: elapsed,
		Left:       a.copyHandState(a.left),
		Right:      a.copyHandState(a.right),
		Combined:   combined,
		Paused:     a.paused,
	}
}

// copyHandState creates a copy of HandState for safe external use.
func (a *Analyzer) copyHandState(h *HandState) *HandState {
	breakdown := make(map[string]int)
	for k, v := range h.PunchBreakdown {
		breakdown[k] = v
	}

	punches := make([]PunchEvent, len(h.RecentPunches))
	copy(punches, h.RecentPunches)

	return &HandState{
		Connected:      h.Connected,
		Calibrated:     h.Calibrated,
		Battery:        h.Battery,
		PacketLoss:     h.PacketLoss,
		PunchCount:     h.PunchCount,
		PunchBreakdown: breakdown,
		MaxForce:       h.MaxForce,
		AvgForce:       h.AvgForce,
		PunchesPerMin:  h.PunchesPerMin,
		RecentPunches:  punches,
		CurrentAccel:   h.CurrentAccel,
		CurrentGyro:    h.CurrentGyro,
	}
}

// broadcastLocked sends state to the handler.
// Must be called with a.mu held.
func (a *Analyzer) broadcastLocked() {
	if a.onState != nil {
		state := a.buildStateLocked()
		// Call handler outside of lock to prevent deadlocks
		go a.onState(state)
	}
}
