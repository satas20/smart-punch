// Package ble provides BLE Central functionality for FighterLink.
package ble

import (
	"log"
	"time"
)

// ScanConfig holds configuration for device scanning.
type ScanConfig struct {
	// ScanTimeout is how long to scan before giving up (0 = forever)
	ScanTimeout time.Duration
	// RetryDelay is how long to wait before retrying a failed connection
	RetryDelay time.Duration
	// ScanInterval is how often to check for disconnected devices (default 2s)
	ScanInterval time.Duration
	// AutoReconnect enables automatic reconnection on disconnect
	AutoReconnect bool
}

// DefaultScanConfig returns sensible defaults for scanning.
func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		ScanTimeout:   0, // Scan forever
		RetryDelay:    2 * time.Second,
		ScanInterval:  2 * time.Second, // Check every 2 seconds
		AutoReconnect: true,
	}
}

// Scanner manages continuous scanning and reconnection for FighterLink devices.
type Scanner struct {
	central *Central
	config  ScanConfig
	running bool
	stop    chan struct{}
}

// NewScanner creates a new Scanner with the given Central and config.
func NewScanner(central *Central, config ScanConfig) *Scanner {
	return &Scanner{
		central: central,
		config:  config,
		stop:    make(chan struct{}),
	}
}

// Start begins the scanning loop.
// This will continuously scan for devices and attempt to connect.
// If AutoReconnect is enabled, it will restart scanning when a device disconnects.
func (s *Scanner) Start() {
	if s.running {
		return
	}
	s.running = true
	s.stop = make(chan struct{})

	// Register disconnect handler for immediate reconnection
	if s.config.AutoReconnect {
		s.central.SetDisconnectHandler(s.onDisconnect)
	}

	go s.scanLoop()
}

// onDisconnect is called when a glove disconnects.
// It triggers an immediate scan attempt instead of waiting for the next interval.
func (s *Scanner) onDisconnect(hand Hand, deviceName string) {
	if !s.running || !s.config.AutoReconnect {
		return
	}
	log.Printf("Scanner: %s disconnected, initiating reconnection scan...", deviceName)
	// Start scanning immediately
	go s.checkAndScan()
}

// Stop halts the scanning loop.
func (s *Scanner) Stop() {
	if !s.running {
		return
	}
	s.running = false
	close(s.stop)
	s.central.StopScanning()
}

// scanLoop is the main scanning goroutine.
// It periodically checks if any glove is disconnected and starts scanning if needed.
func (s *Scanner) scanLoop() {
	log.Println("Scanner: Starting scan loop (checking every", s.config.ScanInterval, ")")

	ticker := time.NewTicker(s.config.ScanInterval)
	defer ticker.Stop()

	// Do an initial check immediately
	s.checkAndScan()

	for {
		select {
		case <-s.stop:
			log.Println("Scanner: Stopped")
			return
		case <-ticker.C:
			s.checkAndScan()
		}
	}
}

// checkAndScan checks if any gloves need connection and starts scanning if needed.
func (s *Scanner) checkAndScan() {
	needLeft := !s.central.IsConnected(LeftHand)
	needRight := !s.central.IsConnected(RightHand)

	if !needLeft && !needRight {
		// Both connected, nothing to do
		return
	}

	// Build list of needed devices for logging
	var needed []string
	if needLeft {
		needed = append(needed, LeftDeviceName)
	}
	if needRight {
		needed = append(needed, RightDeviceName)
	}
	log.Printf("Scanner: Scanning for gloves (need: %v)", needed)

	// Start scanning
	if err := s.central.StartScanning(); err != nil {
		log.Printf("Scanner: Failed to start scan: %v", err)
	}
}

// WaitForBothGloves blocks until both gloves are connected or the context is cancelled.
func (s *Scanner) WaitForBothGloves(timeout time.Duration) bool {
	start := time.Now()
	for {
		if s.central.BothConnected() {
			return true
		}
		if timeout > 0 && time.Since(start) > timeout {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// WaitForAnyGlove blocks until at least one glove is connected.
func (s *Scanner) WaitForAnyGlove(timeout time.Duration) bool {
	start := time.Now()
	for {
		if s.central.IsConnected(LeftHand) || s.central.IsConnected(RightHand) {
			return true
		}
		if timeout > 0 && time.Since(start) > timeout {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}
