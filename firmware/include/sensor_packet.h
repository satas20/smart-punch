/**
 * FighterLink Sensor Packet Definition
 * 
 * 20-byte binary packet structure for BLE transmission.
 * This header is shared between firmware (C++) and should match
 * the Go server's packet parsing exactly.
 */

#ifndef SENSOR_PACKET_H
#define SENSOR_PACKET_H

#include <stdint.h>

/**
 * Binary sensor packet structure (20 bytes total)
 * 
 * All multi-byte values are little-endian (native for ESP32 and x86/ARM)
 * 
 * Field      | Offset | Size | Type   | Scale  | Units
 * -----------|--------|------|--------|--------|-------
 * accX       | 0      | 2    | int16  | ÷100   | m/s²
 * accY       | 2      | 2    | int16  | ÷100   | m/s²
 * accZ       | 4      | 2    | int16  | ÷100   | m/s²
 * gyroX      | 6      | 2    | int16  | ÷10    | °/s
 * gyroY      | 8      | 2    | int16  | ÷10    | °/s
 * gyroZ      | 10     | 2    | int16  | ÷10    | °/s
 * timestamp  | 12     | 4    | uint32 | -      | ms
 * sequence   | 16     | 2    | uint16 | -      | counter
 * battery    | 18     | 1    | uint8  | -      | 0-100%
 * flags      | 19     | 1    | uint8  | -      | bitfield
 * 
 * Flags bitfield:
 *   Bit 0: isCharging (1 = charging, 0 = on battery)
 *   Bit 1: isCalibrated (1 = calibration complete)
 *   Bit 2-7: Reserved
 */
struct __attribute__((packed)) SensorPacket {
    int16_t  accX;       // Accelerometer X (m/s² * 100)
    int16_t  accY;       // Accelerometer Y (m/s² * 100)
    int16_t  accZ;       // Accelerometer Z (m/s² * 100)
    int16_t  gyroX;      // Gyroscope X (°/s * 10)
    int16_t  gyroY;      // Gyroscope Y (°/s * 10)
    int16_t  gyroZ;      // Gyroscope Z (°/s * 10)
    uint32_t timestamp;  // millis() timestamp
    uint16_t sequence;   // Packet sequence number (wraps at 65535)
    uint8_t  battery;    // Battery percentage (0-100)
    uint8_t  flags;      // Status flags
};

// Compile-time size check
static_assert(sizeof(SensorPacket) == 20, "SensorPacket must be exactly 20 bytes");

#endif // SENSOR_PACKET_H
