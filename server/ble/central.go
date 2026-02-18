// Package ble provides BLE Central functionality for FighterLink.
package ble

import (
	"fmt"
	"log"
	"sync"

	"tinygo.org/x/bluetooth"
)

// Hand represents left or right hand.
type Hand int

const (
	LeftHand  Hand = 0
	RightHand Hand = 1
)

func (h Hand) String() string {
	if h == LeftHand {
		return "left"
	}
	return "right"
}

// BLE UUIDs matching the firmware
var (
	ServiceUUID     = bluetooth.NewUUID([16]byte{0xfb, 0x34, 0x9b, 0x5f, 0x80, 0x00, 0x00, 0x80, 0x00, 0x10, 0x00, 0x00, 0x34, 0x12, 0x00, 0x00})
	SensorCharUUID  = bluetooth.NewUUID([16]byte{0xfb, 0x34, 0x9b, 0x5f, 0x80, 0x00, 0x00, 0x80, 0x00, 0x10, 0x00, 0x00, 0x35, 0x12, 0x00, 0x00})
	BatteryCharUUID = bluetooth.NewUUID([16]byte{0xfb, 0x34, 0x9b, 0x5f, 0x80, 0x00, 0x00, 0x80, 0x00, 0x10, 0x00, 0x00, 0x36, 0x12, 0x00, 0x00})
	DeviceCharUUID  = bluetooth.NewUUID([16]byte{0xfb, 0x34, 0x9b, 0x5f, 0x80, 0x00, 0x00, 0x80, 0x00, 0x10, 0x00, 0x00, 0x37, 0x12, 0x00, 0x00})
)

// Device names for scanning
const (
	LeftDeviceName  = "FighterLink_L"
	RightDeviceName = "FighterLink_R"
)

// GloveConnection represents a connected glove.
type GloveConnection struct {
	Hand       Hand
	Device     *bluetooth.Device
	Address    bluetooth.Address
	SensorChar bluetooth.DeviceCharacteristic
	Connected  bool
	LastSeq    uint16
	PacketLoss float64
}

// PacketHandler is called when a sensor packet is received.
type PacketHandler func(hand Hand, packet *SensorPacket)

// Central manages BLE connections to FighterLink gloves.
type Central struct {
	adapter *bluetooth.Adapter
	mu      sync.RWMutex

	leftGlove  *GloveConnection
	rightGlove *GloveConnection

	onPacket PacketHandler
	scanning bool
	stopScan chan struct{}
}

// NewCentral creates a new BLE Central manager.
func NewCentral() *Central {
	return &Central{
		adapter:  bluetooth.DefaultAdapter,
		stopScan: make(chan struct{}),
	}
}

// SetPacketHandler sets the callback for incoming sensor packets.
func (c *Central) SetPacketHandler(handler PacketHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPacket = handler
}

// Enable initializes the BLE adapter.
func (c *Central) Enable() error {
	log.Println("BLE: Enabling adapter...")
	if err := c.adapter.Enable(); err != nil {
		return fmt.Errorf("failed to enable BLE adapter: %w", err)
	}
	log.Println("BLE: Adapter enabled")
	return nil
}

// GetGlove returns the connection for a specific hand.
func (c *Central) GetGlove(hand Hand) *GloveConnection {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if hand == LeftHand {
		return c.leftGlove
	}
	return c.rightGlove
}

// IsConnected returns true if the specified hand is connected.
func (c *Central) IsConnected(hand Hand) bool {
	glove := c.GetGlove(hand)
	return glove != nil && glove.Connected
}

// BothConnected returns true if both gloves are connected.
func (c *Central) BothConnected() bool {
	return c.IsConnected(LeftHand) && c.IsConnected(RightHand)
}

// handleNotification processes incoming BLE notifications.
func (c *Central) handleNotification(hand Hand) func([]byte) {
	return func(data []byte) {
		packet, err := ParsePacket(data)
		if err != nil {
			log.Printf("BLE: Failed to parse packet from %s: %v", hand, err)
			return
		}

		// Track packet loss via sequence numbers
		c.mu.Lock()
		glove := c.leftGlove
		if hand == RightHand {
			glove = c.rightGlove
		}
		if glove != nil && glove.LastSeq > 0 {
			expected := glove.LastSeq + 1
			if packet.Sequence != expected && packet.Sequence != 0 {
				// Calculate packet loss (simple approximation)
				missed := int(packet.Sequence) - int(expected)
				if missed > 0 && missed < 100 {
					glove.PacketLoss = float64(missed) / float64(packet.Sequence) * 100
				}
			}
		}
		if glove != nil {
			glove.LastSeq = packet.Sequence
		}
		handler := c.onPacket
		c.mu.Unlock()

		// Call the packet handler
		if handler != nil {
			handler(hand, packet)
		}
	}
}

// connectToDevice establishes a connection to a discovered glove.
func (c *Central) connectToDevice(result bluetooth.ScanResult, hand Hand) error {
	log.Printf("BLE: Connecting to %s (%s)...", result.LocalName(), result.Address.String())

	device, err := c.adapter.Connect(result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	log.Printf("BLE: Connected to %s, discovering services...", result.LocalName())

	// Discover services
	services, err := device.DiscoverServices([]bluetooth.UUID{ServiceUUID})
	if err != nil {
		device.Disconnect()
		return fmt.Errorf("failed to discover services: %w", err)
	}

	if len(services) == 0 {
		device.Disconnect()
		return fmt.Errorf("FighterLink service not found")
	}

	service := services[0]

	// Discover characteristics
	chars, err := service.DiscoverCharacteristics([]bluetooth.UUID{SensorCharUUID})
	if err != nil {
		device.Disconnect()
		return fmt.Errorf("failed to discover characteristics: %w", err)
	}

	if len(chars) == 0 {
		device.Disconnect()
		return fmt.Errorf("sensor characteristic not found")
	}

	sensorChar := chars[0]

	// Create glove connection
	glove := &GloveConnection{
		Hand:       hand,
		Device:     device,
		Address:    result.Address,
		SensorChar: sensorChar,
		Connected:  true,
	}

	// Enable notifications
	if err := sensorChar.EnableNotifications(c.handleNotification(hand)); err != nil {
		device.Disconnect()
		return fmt.Errorf("failed to enable notifications: %w", err)
	}

	// Store the connection
	c.mu.Lock()
	if hand == LeftHand {
		c.leftGlove = glove
	} else {
		c.rightGlove = glove
	}
	c.mu.Unlock()

	log.Printf("BLE: %s glove connected and streaming", hand)
	return nil
}

// StartScanning begins scanning for FighterLink devices.
func (c *Central) StartScanning() error {
	c.mu.Lock()
	if c.scanning {
		c.mu.Unlock()
		return nil
	}
	c.scanning = true
	c.stopScan = make(chan struct{})
	c.mu.Unlock()

	log.Println("BLE: Starting scan for FighterLink devices...")

	go func() {
		err := c.adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
			name := result.LocalName()

			// Check if this is a FighterLink device we need
			var hand Hand
			var needsConnection bool

			switch name {
			case LeftDeviceName:
				hand = LeftHand
				needsConnection = !c.IsConnected(LeftHand)
			case RightDeviceName:
				hand = RightHand
				needsConnection = !c.IsConnected(RightHand)
			default:
				return // Not a FighterLink device
			}

			if !needsConnection {
				return // Already connected
			}

			log.Printf("BLE: Found %s at %s", name, result.Address.String())

			// Stop scanning temporarily to connect
			adapter.StopScan()

			if err := c.connectToDevice(result, hand); err != nil {
				log.Printf("BLE: Failed to connect to %s: %v", name, err)
			}

			// Resume scanning if we still need devices
			if !c.BothConnected() {
				c.mu.Lock()
				if c.scanning {
					c.mu.Unlock()
					adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
						// This nested scan will be handled by the outer function logic
					})
				} else {
					c.mu.Unlock()
				}
			} else {
				log.Println("BLE: Both gloves connected, stopping scan")
				c.mu.Lock()
				c.scanning = false
				c.mu.Unlock()
			}
		})

		if err != nil {
			log.Printf("BLE: Scan error: %v", err)
		}
	}()

	return nil
}

// StopScanning stops the BLE scan.
func (c *Central) StopScanning() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.scanning {
		c.scanning = false
		c.adapter.StopScan()
		log.Println("BLE: Scan stopped")
	}
}

// Disconnect disconnects from a specific glove.
func (c *Central) Disconnect(hand Hand) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var glove *GloveConnection
	if hand == LeftHand {
		glove = c.leftGlove
		c.leftGlove = nil
	} else {
		glove = c.rightGlove
		c.rightGlove = nil
	}

	if glove != nil && glove.Connected {
		glove.Connected = false
		if err := glove.Device.Disconnect(); err != nil {
			return fmt.Errorf("failed to disconnect %s glove: %w", hand, err)
		}
		log.Printf("BLE: %s glove disconnected", hand)
	}

	return nil
}

// DisconnectAll disconnects from all gloves.
func (c *Central) DisconnectAll() {
	c.Disconnect(LeftHand)
	c.Disconnect(RightHand)
}

// GetBatteryLevel returns the battery level for a glove (if available).
func (c *Central) GetBatteryLevel(hand Hand) (uint8, bool) {
	glove := c.GetGlove(hand)
	if glove == nil || !glove.Connected {
		return 0, false
	}
	// Battery is included in each packet, so we track it in the analyzer
	return 0, false
}
