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
	punchThreshold = 25.0 // m/s² - acceleration above gravity for punch detection
	debounceMS     = 300  // milliseconds between valid punches

	// Punch classification thresholds (gyroscope-based)
	hookGyroThresh     = 200.0 // °/s - rotation around "up" axis for hook detection
	uppercutGyroThresh = 150.0 // °/s - rotation for uppercut detection
	straightGyroMax    = 150.0 // °/s - max rotation for straight punch

	// Stats tracking
	maxRecentPunches = 50  // punches kept in history
	rollingBufSize   = 500 // 5 seconds at 100Hz

	// Calibration constants
	calibrationDuration   = 3.0 // seconds of stillness required
	calibrationSampleRate = 100 // samples per second (100Hz)
	calibrationSamples    = 300 // 3 seconds * 100Hz
	stillnessAccelThresh  = 0.5 // m/s² - max acceleration variance to be "still"
	stillnessGyroThresh   = 5.0 // °/s - max gyro variance to be "still"
	calibrationBufferSize = 50  // samples for variance calculation
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
	CurrentAccel [3]float64 `json:"current_accel"` // X, Y, Z in m/s²
	CurrentGyro  [3]float64 `json:"current_gyro"`  // X, Y, Z in °/s

	// Calibration state
	CalibrationProgress float64    `json:"calibration_progress"` // 0.0 to 1.0
	GravityRef          [3]float64 `json:"gravity_ref"`          // Gravity vector in sensor frame
	GloveOrientation    string     `json:"glove_orientation"`    // "palm_down", "palm_up", etc.
	UpAxis              int        `json:"up_axis"`              // 0=X, 1=Y, 2=Z - which axis points up

	// Internal state
	forceSum          float64      // sum of all punch forces
	lastPunchTS       int64        // last punch timestamp (device)
	lastPunchTime     time.Time    // last punch time (local)
	calibrationBuffer [][6]float64 // rolling buffer for stillness detection [ax,ay,az,gx,gy,gz]
	stillnessCounter  int          // consecutive "still" samples
	serverCalibrated  bool         // true when server has captured gravity reference
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

	// Update battery status
	state.Battery = packet.Battery

	// Get acceleration and gyroscope values
	ax, ay, az := packet.AccelMS2()
	gx, gy, gz := packet.GyroDPS()

	// Store current sensor values (for logging/dashboard)
	state.CurrentAccel = [3]float64{ax, ay, az}
	state.CurrentGyro = [3]float64{gx, gy, gz}

	// ─── Calibration Phase ───────────────────────────────────────────────────
	// Add sample to calibration buffer
	state.calibrationBuffer = append(state.calibrationBuffer, [6]float64{ax, ay, az, gx, gy, gz})
	if len(state.calibrationBuffer) > calibrationSamples {
		state.calibrationBuffer = state.calibrationBuffer[1:] // Keep last N samples
	}

	// Server-side calibration: detect stillness and capture gravity reference
	if !state.serverCalibrated {
		if isStill(state.calibrationBuffer) {
			state.stillnessCounter++

			// Update progress (0.0 to 1.0)
			state.CalibrationProgress = float64(state.stillnessCounter) / float64(calibrationSamples)
			if state.CalibrationProgress > 1.0 {
				state.CalibrationProgress = 1.0
			}

			// After 3 seconds of stillness (300 samples at 100Hz)
			if state.stillnessCounter >= calibrationSamples {
				state.GravityRef = captureGravityReference(state.calibrationBuffer)
				state.UpAxis, state.GloveOrientation = detectOrientation(state.GravityRef)
				state.serverCalibrated = true
				state.Calibrated = true

				// Log calibration completion
				_ = handName // Used in format string below
				// log.Printf not available here, will broadcast state change
			}
		} else {
			// Movement detected, reset stillness counter
			if state.stillnessCounter > 0 {
				state.stillnessCounter = 0
				state.CalibrationProgress = 0
			}
		}

		// Broadcast calibration progress
		a.broadcastLocked()
		return // Don't process punches until calibrated
	}

	// Update calibration status from firmware (for display)
	state.Calibrated = state.serverCalibrated

	// ─── Punch Detection Phase ───────────────────────────────────────────────
	// Skip punch analysis if session not active or paused
	if !a.active || a.paused {
		return
	}

	// Calculate gravity-compensated acceleration magnitude
	punchAx := ax - state.GravityRef[0]
	punchAy := ay - state.GravityRef[1]
	punchAz := az - state.GravityRef[2]
	mag := math.Sqrt(punchAx*punchAx + punchAy*punchAy + punchAz*punchAz)

	// Punch detection: threshold + debounce
	timeSinceLast := int64(packet.Timestamp) - state.lastPunchTS
	if mag > punchThreshold && timeSinceLast > debounceMS {
		// Classify punch type based on gyroscope data and calibrated up axis
		punchType := classifyPunch(gx, gy, gz, state.UpAxis)

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

// classifyPunch determines the punch type based on motion data and calibration.
func classifyPunch(gx, gy, gz float64, upAxis int) PunchType {
	absGX := math.Abs(gx)
	absGY := math.Abs(gy)
	absGZ := math.Abs(gz)

	// Get rotation around the "up" axis (determined during calibration)
	var upRotation float64
	switch upAxis {
	case 0: // X is up
		upRotation = absGX
	case 1: // Y is up
		upRotation = absGY
	case 2: // Z is up (most common for wrist-mounted, palm down)
		upRotation = absGZ
	default:
		upRotation = absGZ // fallback
	}

	// Hook: High rotation around vertical (up) axis - horizontal spinning motion
	if upRotation > hookGyroThresh {
		return PunchHook
	}

	// Uppercut: High rotation around horizontal axes (pitch/roll)
	// When palm is down, uppercut involves X or Y rotation
	var horizontalRotation float64
	switch upAxis {
	case 0: // X is up, so Y and Z are horizontal
		horizontalRotation = math.Max(absGY, absGZ)
	case 1: // Y is up, so X and Z are horizontal
		horizontalRotation = math.Max(absGX, absGZ)
	case 2: // Z is up, so X and Y are horizontal
		horizontalRotation = math.Max(absGX, absGY)
	default:
		horizontalRotation = math.Max(absGX, absGY)
	}

	if horizontalRotation > uppercutGyroThresh {
		return PunchUppercut
	}

	// Straight: Low rotation overall
	maxRotation := math.Max(absGX, math.Max(absGY, absGZ))
	if maxRotation < straightGyroMax {
		return PunchStraight
	}

	return PunchUnknown
}

// ─── Calibration Functions ───────────────────────────────────────────────────

// isStill checks if the recent samples indicate the glove is stationary
func isStill(buffer [][6]float64) bool {
	if len(buffer) < calibrationBufferSize {
		return false
	}

	// Use last N samples for variance calculation
	samples := buffer[len(buffer)-calibrationBufferSize:]

	// Calculate mean for each axis
	var meanAx, meanAy, meanAz, meanGx, meanGy, meanGz float64
	for _, s := range samples {
		meanAx += s[0]
		meanAy += s[1]
		meanAz += s[2]
		meanGx += s[3]
		meanGy += s[4]
		meanGz += s[5]
	}
	n := float64(len(samples))
	meanAx /= n
	meanAy /= n
	meanAz /= n
	meanGx /= n
	meanGy /= n
	meanGz /= n

	// Calculate variance
	var varAx, varAy, varAz, varGx, varGy, varGz float64
	for _, s := range samples {
		varAx += (s[0] - meanAx) * (s[0] - meanAx)
		varAy += (s[1] - meanAy) * (s[1] - meanAy)
		varAz += (s[2] - meanAz) * (s[2] - meanAz)
		varGx += (s[3] - meanGx) * (s[3] - meanGx)
		varGy += (s[4] - meanGy) * (s[4] - meanGy)
		varGz += (s[5] - meanGz) * (s[5] - meanGz)
	}
	varAx /= n
	varAy /= n
	varAz /= n
	varGx /= n
	varGy /= n
	varGz /= n

	// Check if variance is below threshold
	accelVar := math.Sqrt(varAx + varAy + varAz)
	gyroVar := math.Sqrt(varGx + varGy + varGz)

	return accelVar < stillnessAccelThresh && gyroVar < stillnessGyroThresh
}

// captureGravityReference averages recent accelerometer readings to get gravity vector
func captureGravityReference(buffer [][6]float64) [3]float64 {
	if len(buffer) < calibrationBufferSize {
		return [3]float64{0, 0, 9.81} // default: Z-down
	}

	samples := buffer[len(buffer)-calibrationBufferSize:]

	var sumAx, sumAy, sumAz float64
	for _, s := range samples {
		sumAx += s[0]
		sumAy += s[1]
		sumAz += s[2]
	}
	n := float64(len(samples))

	return [3]float64{sumAx / n, sumAy / n, sumAz / n}
}

// detectOrientation determines which axis points "up" based on gravity reference
func detectOrientation(gravityRef [3]float64) (upAxis int, orientation string) {
	absX := math.Abs(gravityRef[0])
	absY := math.Abs(gravityRef[1])
	absZ := math.Abs(gravityRef[2])

	// Find which axis has the most gravity (points up or down)
	if absZ >= absX && absZ >= absY {
		upAxis = 2
		if gravityRef[2] > 0 {
			orientation = "palm_down" // Z+ is up, so palm faces down
		} else {
			orientation = "palm_up" // Z- is up, so palm faces up
		}
	} else if absY >= absX {
		upAxis = 1
		if gravityRef[1] > 0 {
			orientation = "fingers_down"
		} else {
			orientation = "fingers_up"
		}
	} else {
		upAxis = 0
		if gravityRef[0] > 0 {
			orientation = "thumb_down"
		} else {
			orientation = "thumb_up"
		}
	}

	return upAxis, orientation
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
		Connected:           h.Connected,
		Calibrated:          h.Calibrated,
		Battery:             h.Battery,
		PacketLoss:          h.PacketLoss,
		PunchCount:          h.PunchCount,
		PunchBreakdown:      breakdown,
		MaxForce:            h.MaxForce,
		AvgForce:            h.AvgForce,
		PunchesPerMin:       h.PunchesPerMin,
		RecentPunches:       punches,
		CurrentAccel:        h.CurrentAccel,
		CurrentGyro:         h.CurrentGyro,
		CalibrationProgress: h.CalibrationProgress,
		GravityRef:          h.GravityRef,
		GloveOrientation:    h.GloveOrientation,
		UpAxis:              h.UpAxis,
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

// ResetCalibration clears calibration state for a hand, allowing re-calibration.
func (a *Analyzer) ResetCalibration(hand ble.Hand) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var state *HandState
	if hand == ble.LeftHand {
		state = a.left
	} else {
		state = a.right
	}

	// Reset calibration state
	state.serverCalibrated = false
	state.Calibrated = false
	state.CalibrationProgress = 0
	state.stillnessCounter = 0
	state.calibrationBuffer = nil
	state.GravityRef = [3]float64{0, 0, 0}
	state.GloveOrientation = ""
	state.UpAxis = 0

	a.broadcastLocked()
}

// IsCalibrated returns whether a hand is calibrated.
func (a *Analyzer) IsCalibrated(hand ble.Hand) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if hand == ble.LeftHand {
		return a.left.serverCalibrated
	}
	return a.right.serverCalibrated
}

// AnyCalibrated returns true if at least one glove is calibrated.
func (a *Analyzer) AnyCalibrated() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.left.serverCalibrated || a.right.serverCalibrated
}
