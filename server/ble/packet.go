// Package ble provides BLE Central functionality for FighterLink.
package ble

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// PacketSize is the expected size of a sensor packet in bytes.
const PacketSize = 20

// SensorPacket represents the 20-byte binary packet from a FighterLink glove.
// All multi-byte fields are little-endian.
type SensorPacket struct {
	AccX      int16  // Accelerometer X (raw value, divide by 100 for m/s²)
	AccY      int16  // Accelerometer Y
	AccZ      int16  // Accelerometer Z
	GyroX     int16  // Gyroscope X (raw value, divide by 10 for °/s)
	GyroY     int16  // Gyroscope Y
	GyroZ     int16  // Gyroscope Z
	Timestamp uint32 // Milliseconds since boot
	Sequence  uint16 // Packet sequence number
	Battery   uint8  // Battery percentage (0-100)
	Flags     uint8  // Status flags
}

// Flag bit positions
const (
	FlagCharging   uint8 = 1 << 0 // Bit 0: Is charging
	FlagCalibrated uint8 = 1 << 1 // Bit 1: Calibration complete
)

// ErrInvalidPacketSize is returned when the packet data is not 20 bytes.
var ErrInvalidPacketSize = errors.New("invalid packet size: expected 20 bytes")

// ParsePacket decodes a 20-byte binary packet into a SensorPacket struct.
func ParsePacket(data []byte) (*SensorPacket, error) {
	if len(data) != PacketSize {
		return nil, fmt.Errorf("%w, got %d", ErrInvalidPacketSize, len(data))
	}

	p := &SensorPacket{
		AccX:      int16(binary.LittleEndian.Uint16(data[0:2])),
		AccY:      int16(binary.LittleEndian.Uint16(data[2:4])),
		AccZ:      int16(binary.LittleEndian.Uint16(data[4:6])),
		GyroX:     int16(binary.LittleEndian.Uint16(data[6:8])),
		GyroY:     int16(binary.LittleEndian.Uint16(data[8:10])),
		GyroZ:     int16(binary.LittleEndian.Uint16(data[10:12])),
		Timestamp: binary.LittleEndian.Uint32(data[12:16]),
		Sequence:  binary.LittleEndian.Uint16(data[16:18]),
		Battery:   data[18],
		Flags:     data[19],
	}

	return p, nil
}

// AccelMS2 returns accelerometer values in m/s².
func (p *SensorPacket) AccelMS2() (x, y, z float64) {
	return float64(p.AccX) / 100.0,
		float64(p.AccY) / 100.0,
		float64(p.AccZ) / 100.0
}

// GyroDPS returns gyroscope values in degrees per second.
func (p *SensorPacket) GyroDPS() (x, y, z float64) {
	return float64(p.GyroX) / 10.0,
		float64(p.GyroY) / 10.0,
		float64(p.GyroZ) / 10.0
}

// IsCharging returns true if the glove is currently charging.
func (p *SensorPacket) IsCharging() bool {
	return p.Flags&FlagCharging != 0
}

// IsCalibrated returns true if the sensor has completed calibration.
func (p *SensorPacket) IsCalibrated() bool {
	return p.Flags&FlagCalibrated != 0
}

// String returns a human-readable representation of the packet.
func (p *SensorPacket) String() string {
	ax, ay, az := p.AccelMS2()
	gx, gy, gz := p.GyroDPS()
	return fmt.Sprintf(
		"Accel(%.2f, %.2f, %.2f) m/s² | Gyro(%.1f, %.1f, %.1f) °/s | ts=%d seq=%d bat=%d%% flags=0x%02x",
		ax, ay, az, gx, gy, gz, p.Timestamp, p.Sequence, p.Battery, p.Flags,
	)
}
