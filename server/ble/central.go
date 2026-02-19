// Package ble provides BLE Central functionality for FighterLink.
package ble

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/muka/go-bluetooth/bluez"
	"github.com/muka/go-bluetooth/bluez/profile/gatt"
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

// Standard big-endian UUID strings as BlueZ returns them in GetManagedObjects.
// bluetooth.UUID.String() outputs little-endian bytes and does NOT match these.
const (
	serviceUUIDStr    = "00001234-0000-1000-8000-00805f9b34fb"
	sensorCharUUIDStr = "00001235-0000-1000-8000-00805f9b34fb"
)

// GloveConnection represents a connected glove.
type GloveConnection struct {
	Hand       Hand
	Device     *bluetooth.Device
	Address    bluetooth.Address
	SensorChar *gatt.GattCharacteristic1
	PropCh     chan *bluez.PropertyChanged
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

// waitForServicesResolved blocks until BlueZ reports ServicesResolved = true
// for the given device address, or until the timeout expires.
//
// BlueZ performs GATT service discovery asynchronously after the ACL connection
// is established. The ServicesResolved property on the Device1 D-Bus object
// transitions false → true when the GATT profile is fully resolved. Polling
// DiscoverServices before this event yields an empty list even on success.
func waitForServicesResolved(addr bluetooth.Address, timeout time.Duration) error {
	// Derive the BlueZ D-Bus object path from the MAC address.
	// e.g. "D4:E9:F4:E2:B5:8A" → "/org/bluez/hci0/dev_D4_E9_F4_E2_B5_8A"
	mac := strings.ToUpper(addr.String())
	devID := strings.ReplaceAll(mac, ":", "_")
	devPath := dbus.ObjectPath("/org/bluez/hci0/dev_" + devID)

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return fmt.Errorf("dbus: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.bluez", devPath)

	// Fast path: already resolved (e.g. reconnect after prior session).
	v, err := obj.GetProperty("org.bluez.Device1.ServicesResolved")
	if err == nil {
		if resolved, ok := v.Value().(bool); ok && resolved {
			return nil
		}
	}

	// Subscribe to PropertiesChanged signals on this device object.
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchObjectPath(devPath),
	); err != nil {
		return fmt.Errorf("dbus match: %w", err)
	}

	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case sig, ok := <-ch:
			if !ok {
				return fmt.Errorf("dbus signal channel closed")
			}
			if len(sig.Body) < 2 {
				continue
			}
			iface, ok := sig.Body[0].(string)
			if !ok || iface != "org.bluez.Device1" {
				continue
			}
			changed, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				continue
			}
			if v, ok := changed["ServicesResolved"]; ok {
				if resolved, ok := v.Value().(bool); ok && resolved {
					return nil
				}
			}
		case <-timer.C:
			return fmt.Errorf("timeout waiting for ServicesResolved")
		}
	}
}

// discoverGATT opens a fresh D-Bus connection and calls GetManagedObjects
// directly on org.bluez, bypassing the go-bluetooth singleton ObjectManager
// which can return a stale/incomplete view of the GATT object tree.
//
// It returns the GattCharacteristic1 for the given (serviceUUID, charUUID) pair
// under the device identified by addr.
func discoverGATT(addr bluetooth.Address, serviceUUIDStr, charUUIDStr string) (*gatt.GattCharacteristic1, error) {
	// Build device D-Bus path from MAC address.
	// e.g. "D4:E9:F4:E2:B5:8A" → "/org/bluez/hci0/dev_D4_E9_F4_E2_B5_8A"
	mac := strings.ToUpper(addr.String())
	devID := strings.ReplaceAll(mac, ":", "_")
	devPath := "/org/bluez/hci0/dev_" + devID

	serviceUUIDStr = strings.ToLower(serviceUUIDStr)
	charUUIDStr = strings.ToLower(charUUIDStr)

	// Open a fresh D-Bus connection — NOT the go-bluetooth singleton.
	// The singleton's cached connection may return stale GetManagedObjects data.
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer conn.Close()

	// Call GetManagedObjects on the root BlueZ ObjectManager.
	obj := conn.Object("org.bluez", "/")
	var managed map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	if err := obj.Call("org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&managed); err != nil {
		return nil, fmt.Errorf("GetManagedObjects: %w", err)
	}

	log.Printf("BLE: GetManagedObjects returned %d total objects", len(managed))

	// Find the service path under our device.
	var servicePath string
	for path, ifaces := range managed {
		pathStr := string(path)

		// Must be exactly one level under devPath: devPath/serviceXXXX
		if !strings.HasPrefix(pathStr, devPath+"/service") {
			continue
		}
		suffix := pathStr[len(devPath)+1:] // e.g. "service0028"
		if strings.Contains(suffix, "/") {
			continue // deeper nesting — skip
		}

		svcIface, ok := ifaces["org.bluez.GattService1"]
		if !ok {
			continue
		}
		uuidVar, ok := svcIface["UUID"]
		if !ok {
			continue
		}
		uuid, ok := uuidVar.Value().(string)
		if !ok {
			continue
		}
		log.Printf("BLE: Service candidate %s UUID=%s", pathStr, uuid)
		if strings.ToLower(uuid) == serviceUUIDStr {
			servicePath = pathStr
			log.Printf("BLE: Matched service at %s", servicePath)
			break
		}
	}

	if servicePath == "" {
		// Emit everything we found under this device for diagnostics.
		for path := range managed {
			if strings.HasPrefix(string(path), devPath) {
				log.Printf("BLE: Object under device: %s", path)
			}
		}
		return nil, fmt.Errorf("service %s not found on %s", serviceUUIDStr, devPath)
	}

	// Find the sensor characteristic under the matched service.
	var charPath string
	for path, ifaces := range managed {
		pathStr := string(path)

		// Must be exactly one level under servicePath: servicePath/charXXXX
		if !strings.HasPrefix(pathStr, servicePath+"/char") {
			continue
		}
		suffix := pathStr[len(servicePath)+1:] // e.g. "char0029"
		if strings.Contains(suffix, "/") {
			continue
		}

		charIface, ok := ifaces["org.bluez.GattCharacteristic1"]
		if !ok {
			continue
		}
		uuidVar, ok := charIface["UUID"]
		if !ok {
			continue
		}
		uuid, ok := uuidVar.Value().(string)
		if !ok {
			continue
		}
		log.Printf("BLE: Char candidate %s UUID=%s", pathStr, uuid)
		if strings.ToLower(uuid) == charUUIDStr {
			charPath = pathStr
			log.Printf("BLE: Matched characteristic at %s", charPath)
			break
		}
	}

	if charPath == "" {
		return nil, fmt.Errorf("characteristic %s not found under %s", charUUIDStr, servicePath)
	}

	// Construct the GattCharacteristic1 wrapper.
	// NewGattCharacteristic1 uses the go-bluetooth Client which lazily connects
	// via the singleton D-Bus connection — this is fine for method calls like
	// StartNotify and WatchProperties; only GetManagedObjects was unreliable.
	char, err := gatt.NewGattCharacteristic1(dbus.ObjectPath(charPath))
	if err != nil {
		return nil, fmt.Errorf("NewGattCharacteristic1(%s): %w", charPath, err)
	}

	return char, nil
}

// connectToDevice establishes a connection to a discovered glove.
func (c *Central) connectToDevice(result bluetooth.ScanResult, hand Hand) error {
	log.Printf("BLE: Connecting to %s (%s)...", result.LocalName(), result.Address.String())

	device, err := c.adapter.Connect(result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	log.Printf("BLE: Connected to %s, waiting for GATT profile...", result.LocalName())

	// Wait for BlueZ to complete GATT service discovery (ServicesResolved = true).
	if err := waitForServicesResolved(result.Address, 15*time.Second); err != nil {
		device.Disconnect()
		return fmt.Errorf("GATT not resolved on %s: %w", result.LocalName(), err)
	}

	log.Printf("BLE: GATT resolved, discovering services via direct D-Bus...")

	// Discover the sensor characteristic using our own D-Bus call, bypassing
	// the go-bluetooth singleton ObjectManager.
	sensorChar, err := discoverGATT(result.Address, serviceUUIDStr, sensorCharUUIDStr)
	if err != nil {
		device.Disconnect()
		return fmt.Errorf("GATT discovery failed on %s: %w", result.LocalName(), err)
	}

	// Subscribe to PropertiesChanged signals for this characteristic.
	// This replicates bluetooth.DeviceCharacteristic.EnableNotifications internally.
	propCh, err := sensorChar.WatchProperties()
	if err != nil {
		device.Disconnect()
		return fmt.Errorf("WatchProperties failed: %w", err)
	}

	// Ask BlueZ to start sending GATT notifications from the peripheral.
	if err := sensorChar.StartNotify(); err != nil {
		_ = sensorChar.UnwatchProperties(propCh)
		device.Disconnect()
		return fmt.Errorf("StartNotify failed: %w", err)
	}

	// Create glove connection record.
	glove := &GloveConnection{
		Hand:       hand,
		Device:     device,
		Address:    result.Address,
		SensorChar: sensorChar,
		PropCh:     propCh,
		Connected:  true,
	}

	// Store before launching the goroutine so handleNotification can find it.
	c.mu.Lock()
	if hand == LeftHand {
		c.leftGlove = glove
	} else {
		c.rightGlove = glove
	}
	c.mu.Unlock()

	// Dispatch incoming GATT notifications to the packet handler.
	notifHandler := c.handleNotification(hand)
	go func() {
		for update := range propCh {
			if update == nil {
				continue
			}
			if update.Interface == "org.bluez.GattCharacteristic1" && update.Name == "Value" {
				notifHandler(update.Value.([]byte))
			}
		}
	}()

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
		// Stop notifications and clean up the D-Bus signal subscription.
		if glove.SensorChar != nil {
			_ = glove.SensorChar.StopNotify()
			if glove.PropCh != nil {
				_ = glove.SensorChar.UnwatchProperties(glove.PropCh)
			}
		}
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
