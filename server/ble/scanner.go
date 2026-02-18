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
	// AutoReconnect enables automatic reconnection on disconnect
	AutoReconnect bool
}

// DefaultScanConfig returns sensible defaults for scanning.
func DefaultScanConfig() ScanConfig {
	return ScanConfig{
		ScanTimeout:   0, // Scan forever
		RetryDelay:    2 * time.Second,
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

	go s.scanLoop()
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
func (s *Scanner) scanLoop() {
	log.Println("Scanner: Starting scan loop")

	for {
		select {
		case <-s.stop:
			log.Println("Scanner: Stopped")
			return
		default:
		}

		// Check what we need
		needLeft := !s.central.IsConnected(LeftHand)
		needRight := !s.central.IsConnected(RightHand)

		if !needLeft && !needRight {
			// Both connected, wait a bit then check again
			time.Sleep(time.Second)
			continue
		}

		if needLeft || needRight {
			var needed []string
			if needLeft {
				needed = append(needed, "FighterLink_L")
			}
			if needRight {
				needed = append(needed, "FighterLink_R")
			}
			log.Printf("Scanner: Looking for %v", needed)
		}

		// Start scanning
		if err := s.central.StartScanning(); err != nil {
			log.Printf("Scanner: Failed to start scan: %v", err)
			time.Sleep(s.config.RetryDelay)
			continue
		}

		// Wait for connections or timeout
		if s.config.ScanTimeout > 0 {
			select {
			case <-s.stop:
				return
			case <-time.After(s.config.ScanTimeout):
				s.central.StopScanning()
			}
		} else {
			// Scan indefinitely, but check periodically if we should stop
			ticker := time.NewTicker(time.Second)
			for {
				select {
				case <-s.stop:
					ticker.Stop()
					return
				case <-ticker.C:
					if s.central.BothConnected() {
						ticker.Stop()
						s.central.StopScanning()
						goto connected
					}
				}
			}
		connected:
		}

		// Small delay before next iteration
		time.Sleep(100 * time.Millisecond)
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
