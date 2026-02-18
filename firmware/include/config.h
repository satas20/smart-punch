/**
 * FighterLink Configuration
 * 
 * BLE UUIDs, Pin Definitions, and Constants
 */

#ifndef CONFIG_H
#define CONFIG_H

// ─── Hand Identification ─────────────────────────────────────────────────────
// Change this value before flashing each glove:
//   0 = Left Hand  (advertises as "FighterLink_L")
//   1 = Right Hand (advertises as "FighterLink_R")
#define HAND_ID 0

// ─── BLE Configuration ───────────────────────────────────────────────────────
// Custom UUIDs for FighterLink service and characteristics
#define BLE_SERVICE_UUID        "00001234-0000-1000-8000-00805f9b34fb"
#define BLE_CHAR_SENSOR_UUID    "00001235-0000-1000-8000-00805f9b34fb"  // NOTIFY
#define BLE_CHAR_BATTERY_UUID   "00001236-0000-1000-8000-00805f9b34fb"  // READ, NOTIFY
#define BLE_CHAR_DEVICE_UUID    "00001237-0000-1000-8000-00805f9b34fb"  // READ (hand ID)

// Device names based on hand
#if HAND_ID == 0
    #define BLE_DEVICE_NAME "FighterLink_L"
#else
    #define BLE_DEVICE_NAME "FighterLink_R"
#endif

// ─── Pin Definitions (XIAO ESP32C3) ──────────────────────────────────────────
// I2C for MPU6050
#define PIN_SDA         6   // GPIO6 - I2C Data
#define PIN_SCL         7   // GPIO7 - I2C Clock

// Status LED (onboard)
#define PIN_LED         10  // GPIO10 - Onboard LED (active LOW on XIAO)

// Battery voltage monitoring (optional)
#define PIN_VBAT        2   // GPIO2 - ADC for battery voltage
#define PIN_VCHARGE     3   // GPIO3 - Charging detection (5V from pogo pins)

// ─── Timing Constants ────────────────────────────────────────────────────────
#define SAMPLE_RATE_MS          10      // 10ms = 100Hz sensor sampling
#define BLE_NOTIFY_INTERVAL_MS  10      // Send BLE notification every 10ms
#define BATTERY_UPDATE_MS       5000    // Update battery level every 5 seconds
#define LED_BLINK_FAST_MS       200     // Fast blink period (advertising)
#define LED_BLINK_SLOW_MS       1000    // Slow blink period (initializing)

// ─── Sensor Scaling ──────────────────────────────────────────────────────────
// MPU6050 outputs acceleration in g, we convert to m/s² and scale for int16
// Accelerometer: value * 9.81 * 100 → int16 (divide by 100 on receiver)
// Gyroscope: value * 10 → int16 (divide by 10 on receiver, units: °/s)
#define ACCEL_SCALE     100     // m/s² * 100 → int16
#define GYRO_SCALE      10      // °/s * 10 → int16
#define GRAVITY_MS2     9.81f   // m/s²

// ─── Battery Monitoring ──────────────────────────────────────────────────────
// LiPo voltage range: 3.0V (empty) to 4.2V (full)
// With voltage divider: adjust these based on your circuit
#define VBAT_MIN        3.0f    // Minimum battery voltage (0%)
#define VBAT_MAX        4.2f    // Maximum battery voltage (100%)
#define VCHARGE_THRESH  4.0f    // Voltage threshold for "charging" detection

// ─── Status Flags (bit positions) ────────────────────────────────────────────
#define FLAG_CHARGING       (1 << 0)    // Bit 0: Is charging
#define FLAG_CALIBRATED     (1 << 1)    // Bit 1: Calibration complete
// Bits 2-7: Reserved for future use

// ─── Calibration ─────────────────────────────────────────────────────────────
#define CALIBRATION_SAMPLES 500     // Number of samples for offset calibration

#endif // CONFIG_H
