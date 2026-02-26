/**
 * FighterLink Boxing Glove Firmware
 * 
 * Arduino IDE version for ESP32 DevKit
 * 
 * BLE Peripheral that streams MPU6050 sensor data at 100Hz.
 * Features smart power management with light sleep idle mode.
 * 
 * Hardware: ESP32 DevKit + MPU6050 + TP4056 (LiPo charger)
 *   - MPU6050 SDA → GPIO21
 *   - MPU6050 SCL → GPIO22
 *   - Onboard LED → GPIO2
 *   - BOOT button → GPIO0 (built-in, used for state control)
 * 
 * Setup:
 *   1. Install ESP32 board support in Arduino IDE
 *   2. Install MPU6050_light library
 *   3. Select Board: "ESP32 Dev Module"
 *   4. Set HAND_ID below (0 = Left, 1 = Right)
 *   5. Upload
 * 
 * Power Management (Light Sleep Idle Mode):
 *   - On power-up: Starts BLE advertising for 60 seconds
 *   - If no connection: Goes to IDLE mode (~1mA with light sleep)
 *   - BOOT button press while IDLE: Starts advertising
 *   - BOOT button press while ACTIVE: Goes to IDLE mode
 *   - On disconnect: Tries to reconnect for 60 seconds, then goes to IDLE
 * 
 * LED States:
 *   - Brief pulse every 5s: IDLE mode (low power)
 *   - Fast blink (200ms): Advertising, looking for connection
 *   - Slow blink (1s): Reconnecting after disconnect
 *   - Medium blink (500ms): Connected but not calibrated
 *   - Solid ON: Connected and calibrated, streaming data
 * 
 * Battery Life (150mAh):
 *   - IDLE mode: ~150 hours (6+ days)
 *   - Active streaming: ~1.5-2 hours
 */

// ═══════════════════════════════════════════════════════════════════════════════
// CONFIGURATION - CHANGE THIS FOR EACH GLOVE
// ═══════════════════════════════════════════════════════════════════════════════

// Set to 0 for LEFT glove, 1 for RIGHT glove
#define HAND_ID 0

// ═══════════════════════════════════════════════════════════════════════════════
// INCLUDES
// ═══════════════════════════════════════════════════════════════════════════════

#include <Wire.h>
#include <BLEDevice.h>
#include <BLEServer.h>
#include <BLEUtils.h>
#include <BLE2902.h>
#include <MPU6050_light.h>
#include "esp_sleep.h"

// ═══════════════════════════════════════════════════════════════════════════════
// BLE CONFIGURATION
// ═══════════════════════════════════════════════════════════════════════════════

// BLE UUIDs (must match Go server)
#define BLE_SERVICE_UUID        "00001234-0000-1000-8000-00805f9b34fb"
#define BLE_CHAR_SENSOR_UUID    "00001235-0000-1000-8000-00805f9b34fb"
#define BLE_CHAR_BATTERY_UUID   "00001236-0000-1000-8000-00805f9b34fb"
#define BLE_CHAR_DEVICE_UUID    "00001237-0000-1000-8000-00805f9b34fb"

// Device name based on hand
#if HAND_ID == 0
  #define BLE_DEVICE_NAME "FighterLink_L"
#else
  #define BLE_DEVICE_NAME "FighterLink_R"
#endif

// ═══════════════════════════════════════════════════════════════════════════════
// PIN DEFINITIONS (ESP32 DevKit)
// ═══════════════════════════════════════════════════════════════════════════════

#define PIN_SDA         21      // I2C Data (MPU6050)
#define PIN_SCL         22      // I2C Clock (MPU6050)
#define PIN_LED         2       // Onboard LED (active HIGH on DevKit)
#define PIN_BOOT_BUTTON 0       // BOOT button (GPIO0, active LOW)

// ═══════════════════════════════════════════════════════════════════════════════
// TIMING CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

#define SAMPLE_RATE_MS          10      // 10ms = 100Hz
#define LED_BLINK_FAST_MS       200     // Fast blink (advertising)
#define LED_BLINK_SLOW_MS       1000    // Slow blink (reconnecting)
#define LED_BLINK_CALIBRATING   500     // Medium blink (calibrating)

// ═══════════════════════════════════════════════════════════════════════════════
// POWER MANAGEMENT CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

#define ADVERTISING_TIMEOUT_MS  60000   // 60 seconds to find connection
#define RECONNECT_TIMEOUT_MS    60000   // 60 seconds to reconnect after disconnect
#define BUTTON_DEBOUNCE_MS      50      // Button debounce time

// ═══════════════════════════════════════════════════════════════════════════════
// IDLE MODE CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

#define IDLE_PULSE_INTERVAL_MS  5000    // LED pulse every 5 seconds in idle
#define IDLE_PULSE_DURATION_MS  50      // LED pulse duration (brief flash)
#define IDLE_SLEEP_DURATION_US  100000  // Light sleep for 100ms (in microseconds)

// ═══════════════════════════════════════════════════════════════════════════════
// SMART CALIBRATION CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

#define STILLNESS_WINDOW_MS     3000    // Must be still for 3 seconds to calibrate
#define STILLNESS_SAMPLES       50      // Number of samples for variance calculation
#define STILLNESS_ACCEL_THRESH  0.15f   // Max acceleration variance (m/s²) to be "still"
#define STILLNESS_GYRO_THRESH   5.0f    // Max gyro variance (°/s) to be "still"

// ═══════════════════════════════════════════════════════════════════════════════
// SENSOR SCALING
// ═══════════════════════════════════════════════════════════════════════════════

#define ACCEL_SCALE     100     // m/s² * 100 → int16
#define GYRO_SCALE      10      // °/s * 10 → int16
#define GRAVITY_MS2     9.81f   // m/s²

// ═══════════════════════════════════════════════════════════════════════════════
// STATUS FLAGS
// ═══════════════════════════════════════════════════════════════════════════════

#define FLAG_CHARGING       (1 << 0)
#define FLAG_CALIBRATED     (1 << 1)

// ═══════════════════════════════════════════════════════════════════════════════
// SENSOR PACKET STRUCTURE (20 bytes)
// ═══════════════════════════════════════════════════════════════════════════════

struct __attribute__((packed)) SensorPacket {
    int16_t  accX;       // Accelerometer X (m/s² * 100)
    int16_t  accY;       // Accelerometer Y (m/s² * 100)
    int16_t  accZ;       // Accelerometer Z (m/s² * 100)
    int16_t  gyroX;      // Gyroscope X (°/s * 10)
    int16_t  gyroY;      // Gyroscope Y (°/s * 10)
    int16_t  gyroZ;      // Gyroscope Z (°/s * 10)
    uint32_t timestamp;  // millis() timestamp
    uint16_t sequence;   // Packet sequence number
    uint8_t  battery;    // Battery percentage (0-100)
    uint8_t  flags;      // Status flags
};

// Verify struct size at compile time
static_assert(sizeof(SensorPacket) == 20, "SensorPacket must be 20 bytes");

// ═══════════════════════════════════════════════════════════════════════════════
// STATE MACHINE
// ═══════════════════════════════════════════════════════════════════════════════

enum DeviceState {
    STATE_IDLE,           // Low power idle mode (light sleep, BLE off)
    STATE_ADVERTISING,    // Looking for BLE connection
    STATE_CONNECTED,      // Connected and streaming data
    STATE_RECONNECTING    // Lost connection, trying to reconnect
};

// ═══════════════════════════════════════════════════════════════════════════════
// GLOBAL OBJECTS
// ═══════════════════════════════════════════════════════════════════════════════

MPU6050 mpu(Wire);

BLEServer* pServer = nullptr;
BLECharacteristic* pSensorChar = nullptr;
BLECharacteristic* pBatteryChar = nullptr;
BLECharacteristic* pDeviceChar = nullptr;

// ═══════════════════════════════════════════════════════════════════════════════
// GLOBAL STATE
// ═══════════════════════════════════════════════════════════════════════════════

volatile bool deviceConnected = false;
volatile bool oldDeviceConnected = false;

uint16_t sequenceNumber = 0;
uint32_t lastSampleTime = 0;
uint32_t lastLedToggle = 0;
bool ledState = false;
bool isCalibrated = false;

// Power management state
DeviceState currentState = STATE_ADVERTISING;
uint32_t stateStartTime = 0;
bool wasEverConnected = false;

// ═══════════════════════════════════════════════════════════════════════════════
// SMART CALIBRATION STATE
// ═══════════════════════════════════════════════════════════════════════════════

// Rolling buffer for variance calculation
float accelBuffer[STILLNESS_SAMPLES][3];  // X, Y, Z
float gyroBuffer[STILLNESS_SAMPLES][3];   // X, Y, Z
int bufferIndex = 0;
int bufferCount = 0;

// Stillness tracking
uint32_t stillnessStartTime = 0;
bool wasStill = false;
bool isCurrentlyStill = false;

// ═══════════════════════════════════════════════════════════════════════════════
// BLE CALLBACKS
// ═══════════════════════════════════════════════════════════════════════════════

class ServerCallbacks : public BLEServerCallbacks {
    void onConnect(BLEServer* pServer) override {
        deviceConnected = true;
        Serial.println("BLE: Client connected");
    }

    void onDisconnect(BLEServer* pServer) override {
        deviceConnected = false;
        Serial.println("BLE: Client disconnected");
    }
};

// ═══════════════════════════════════════════════════════════════════════════════
// LED CONTROL (ESP32 DevKit - active HIGH)
// ═══════════════════════════════════════════════════════════════════════════════

void setLed(bool on) {
    digitalWrite(PIN_LED, on ? HIGH : LOW);
    ledState = on;
}

void blinkLed(uint32_t periodMs) {
    uint32_t now = millis();
    if (now - lastLedToggle >= periodMs / 2) {
        setLed(!ledState);
        lastLedToggle = now;
    }
}

// ═══════════════════════════════════════════════════════════════════════════════
// POWER MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════════

// Forward declarations
void changeState(DeviceState newState);
void setupBLE();

void enterIdleMode() {
    Serial.println("Entering idle mode (light sleep)...");
    Serial.println("Press BOOT button to start advertising");
    Serial.flush();
    
    // Turn off LED
    setLed(false);
    
    // Reset connection state BEFORE deinit to prevent stale flags
    deviceConnected = false;
    oldDeviceConnected = false;
    
    // Stop BLE to save power
    BLEDevice::deinit(false);
    
    // Transition to idle state
    changeState(STATE_IDLE);
}

// Check if button is pressed (non-blocking, for idle mode)
bool isBootButtonPressed() {
    return digitalRead(PIN_BOOT_BUTTON) == LOW;
}

// Wake from idle mode and reinitialize BLE
void wakeFromIdle() {
    Serial.println("Waking from idle mode...");
    
    // Wait for button release to avoid immediate re-trigger
    while (digitalRead(PIN_BOOT_BUTTON) == LOW) {
        setLed(true);
        delay(50);
        setLed(false);
        delay(50);
    }
    delay(BUTTON_DEBOUNCE_MS);
    
    // Reset state for fresh connection
    sequenceNumber = 0;
    deviceConnected = false;
    oldDeviceConnected = false;
    
    // Reinitialize BLE
    setupBLE();
    
    // Reset calibration state (may need recalibration after idle)
    // Keep isCalibrated as-is since MPU6050 offsets are still valid
    
    // Transition to advertising
    changeState(STATE_ADVERTISING);
    Serial.println("Now advertising - looking for connection...");
}

// Idle loop with light sleep - called repeatedly while in STATE_IDLE
void handleIdleState() {
    static uint32_t lastPulseTime = 0;
    uint32_t now = millis();
    
    // Check for button press BEFORE sleeping
    if (isBootButtonPressed()) {
        delay(BUTTON_DEBOUNCE_MS);  // Debounce
        if (isBootButtonPressed()) {
            wakeFromIdle();
            return;
        }
    }
    
    // Brief LED pulse every 5 seconds to show device is alive
    if (now - lastPulseTime >= IDLE_PULSE_INTERVAL_MS) {
        setLed(true);
        delay(IDLE_PULSE_DURATION_MS);
        setLed(false);
        lastPulseTime = now;
    }
    
    // Enter light sleep to save power (~1mA vs ~20mA awake)
    // Will wake after 100ms OR on GPIO interrupt
    esp_sleep_enable_timer_wakeup(IDLE_SLEEP_DURATION_US);
    esp_light_sleep_start();
    
    // After waking from light sleep, loop continues
}

bool checkBootButtonPressed() {
    // GPIO0 is active LOW (pressed = LOW)
    if (digitalRead(PIN_BOOT_BUTTON) == LOW) {
        delay(BUTTON_DEBOUNCE_MS);  // Debounce
        if (digitalRead(PIN_BOOT_BUTTON) == LOW) {
            // Wait for button release to avoid immediate re-trigger
            while (digitalRead(PIN_BOOT_BUTTON) == LOW) {
                delay(10);
            }
            return true;
        }
    }
    return false;
}

void changeState(DeviceState newState) {
    const char* stateNames[] = {"IDLE", "ADVERTISING", "CONNECTED", "RECONNECTING"};
    Serial.printf("State: %s -> %s\n", stateNames[currentState], stateNames[newState]);
    
    currentState = newState;
    stateStartTime = millis();
}

// ═══════════════════════════════════════════════════════════════════════════════
// BLE SETUP
// ═══════════════════════════════════════════════════════════════════════════════

void setupBLE() {
    Serial.println("BLE: Initializing...");
    
    // Initialize BLE with device name
    BLEDevice::init(BLE_DEVICE_NAME);
    
    // Create BLE Server
    pServer = BLEDevice::createServer();
    pServer->setCallbacks(new ServerCallbacks());
    
    // Create FighterLink Service
    BLEService* pService = pServer->createService(BLE_SERVICE_UUID);
    
    // Create Sensor Data Characteristic (NOTIFY only)
    pSensorChar = pService->createCharacteristic(
        BLE_CHAR_SENSOR_UUID,
        BLECharacteristic::PROPERTY_NOTIFY
    );
    pSensorChar->addDescriptor(new BLE2902());
    
    // Create Battery Level Characteristic (READ + NOTIFY)
    pBatteryChar = pService->createCharacteristic(
        BLE_CHAR_BATTERY_UUID,
        BLECharacteristic::PROPERTY_READ | BLECharacteristic::PROPERTY_NOTIFY
    );
    pBatteryChar->addDescriptor(new BLE2902());
    uint8_t initBattery = 100;  // Hardcoded for now
    pBatteryChar->setValue(&initBattery, 1);
    
    // Create Device Info Characteristic (READ only - returns hand ID)
    pDeviceChar = pService->createCharacteristic(
        BLE_CHAR_DEVICE_UUID,
        BLECharacteristic::PROPERTY_READ
    );
    uint8_t handId = HAND_ID;
    pDeviceChar->setValue(&handId, 1);
    
    // Start the service
    pService->start();
    
    // Start advertising
    BLEAdvertising* pAdvertising = BLEDevice::getAdvertising();
    pAdvertising->addServiceUUID(BLE_SERVICE_UUID);
    pAdvertising->setScanResponse(true);
    pAdvertising->setMinPreferred(0x06);
    pAdvertising->setMinPreferred(0x12);
    BLEDevice::startAdvertising();
    
    Serial.printf("BLE: Advertising as '%s'\n", BLE_DEVICE_NAME);
}

// ═══════════════════════════════════════════════════════════════════════════════
// MPU6050 SETUP
// ═══════════════════════════════════════════════════════════════════════════════

bool setupMPU() {
    Serial.println("MPU6050: Initializing...");
    
    Wire.begin(PIN_SDA, PIN_SCL);
    
    byte status = mpu.begin();
    if (status != 0) {
        Serial.printf("MPU6050: Init failed with status %d\n", status);
        return false;
    }
    
    Serial.println("MPU6050: Ready (uncalibrated)");
    Serial.println("MPU6050: Hold glove still for 3 seconds to calibrate...");
    
    // NO blocking calibration - we'll auto-calibrate when still
    isCalibrated = false;
    
    return true;
}

// ═══════════════════════════════════════════════════════════════════════════════
// SMART CALIBRATION - Stillness Detection & Auto-Calibrate
// ═══════════════════════════════════════════════════════════════════════════════

// Calculate variance of a buffer
float calculateVariance(float buffer[][3], int count, int axis) {
    if (count < 2) return 999.0f;  // Not enough samples
    
    // Calculate mean
    float mean = 0;
    for (int i = 0; i < count; i++) {
        mean += buffer[i][axis];
    }
    mean /= count;
    
    // Calculate variance
    float variance = 0;
    for (int i = 0; i < count; i++) {
        float diff = buffer[i][axis] - mean;
        variance += diff * diff;
    }
    variance /= count;
    
    return variance;
}

// Check if sensor is currently still (low variance)
bool checkStillness() {
    if (bufferCount < STILLNESS_SAMPLES) {
        return false;  // Not enough samples yet
    }
    
    // Calculate variance for each axis
    float accelVarX = calculateVariance(accelBuffer, bufferCount, 0);
    float accelVarY = calculateVariance(accelBuffer, bufferCount, 1);
    float accelVarZ = calculateVariance(accelBuffer, bufferCount, 2);
    
    float gyroVarX = calculateVariance(gyroBuffer, bufferCount, 0);
    float gyroVarY = calculateVariance(gyroBuffer, bufferCount, 1);
    float gyroVarZ = calculateVariance(gyroBuffer, bufferCount, 2);
    
    // Max variance across all axes
    float maxAccelVar = max(accelVarX, max(accelVarY, accelVarZ));
    float maxGyroVar = max(gyroVarX, max(gyroVarY, gyroVarZ));
    
    // Check if below thresholds
    bool still = (maxAccelVar < STILLNESS_ACCEL_THRESH * STILLNESS_ACCEL_THRESH) &&
                 (maxGyroVar < STILLNESS_GYRO_THRESH * STILLNESS_GYRO_THRESH);
    
    return still;
}

// Add sample to rolling buffer
void addSampleToBuffer(float ax, float ay, float az, float gx, float gy, float gz) {
    accelBuffer[bufferIndex][0] = ax;
    accelBuffer[bufferIndex][1] = ay;
    accelBuffer[bufferIndex][2] = az;
    
    gyroBuffer[bufferIndex][0] = gx;
    gyroBuffer[bufferIndex][1] = gy;
    gyroBuffer[bufferIndex][2] = gz;
    
    bufferIndex = (bufferIndex + 1) % STILLNESS_SAMPLES;
    if (bufferCount < STILLNESS_SAMPLES) {
        bufferCount++;
    }
}

// Run calibration
void performCalibration() {
    Serial.println("MPU6050: Stillness detected - calibrating...");
    
    // Run the MPU6050 library's calibration
    mpu.calcOffsets(true, true);
    
    isCalibrated = true;
    
    Serial.println("MPU6050: Calibration complete!");
    
    // Visual feedback - quick triple blink
    for (int i = 0; i < 3; i++) {
        setLed(true);
        delay(100);
        setLed(false);
        delay(100);
    }
}

// Update stillness detection and trigger calibration if needed
void updateSmartCalibration() {
    // Skip if already calibrated
    if (isCalibrated) {
        return;
    }
    
    // Read current sensor values
    mpu.update();
    
    float ax = mpu.getAccX() * GRAVITY_MS2;
    float ay = mpu.getAccY() * GRAVITY_MS2;
    float az = mpu.getAccZ() * GRAVITY_MS2;
    float gx = mpu.getGyroX();
    float gy = mpu.getGyroY();
    float gz = mpu.getGyroZ();
    
    // Add to rolling buffer
    addSampleToBuffer(ax, ay, az, gx, gy, gz);
    
    // Check stillness
    isCurrentlyStill = checkStillness();
    
    uint32_t now = millis();
    
    if (isCurrentlyStill) {
        if (!wasStill) {
            // Just became still
            stillnessStartTime = now;
            Serial.println("MPU6050: Detecting stillness...");
        }
        
        // Check if still long enough
        uint32_t stillDuration = now - stillnessStartTime;
        if (stillDuration >= STILLNESS_WINDOW_MS) {
            performCalibration();
        }
    } else {
        if (wasStill) {
            // Movement detected, reset
            Serial.println("MPU6050: Movement detected, calibration reset");
        }
        stillnessStartTime = 0;
    }
    
    wasStill = isCurrentlyStill;
}

// ═══════════════════════════════════════════════════════════════════════════════
// SEND SENSOR DATA
// ═══════════════════════════════════════════════════════════════════════════════

void sendSensorData() {
    // Update MPU6050 readings
    mpu.update();
    
    // Build packet
    SensorPacket packet;
    
    // Accelerometer: g → m/s² → scaled int16
    packet.accX = (int16_t)(mpu.getAccX() * GRAVITY_MS2 * ACCEL_SCALE);
    packet.accY = (int16_t)(mpu.getAccY() * GRAVITY_MS2 * ACCEL_SCALE);
    packet.accZ = (int16_t)(mpu.getAccZ() * GRAVITY_MS2 * ACCEL_SCALE);
    
    // Gyroscope: °/s → scaled int16
    packet.gyroX = (int16_t)(mpu.getGyroX() * GYRO_SCALE);
    packet.gyroY = (int16_t)(mpu.getGyroY() * GYRO_SCALE);
    packet.gyroZ = (int16_t)(mpu.getGyroZ() * GYRO_SCALE);
    
    // Timestamp and sequence
    packet.timestamp = millis();
    packet.sequence = sequenceNumber++;
    
    // Battery (hardcoded for now)
    packet.battery = 100;
    
    // Flags
    packet.flags = 0;
    if (isCalibrated) {
        packet.flags |= FLAG_CALIBRATED;
    }
    
    // Send via BLE notification
    pSensorChar->setValue((uint8_t*)&packet, sizeof(SensorPacket));
    pSensorChar->notify();
}

// ═══════════════════════════════════════════════════════════════════════════════
// SETUP
// ═══════════════════════════════════════════════════════════════════════════════

void setup() {
    // ─── CRITICAL: Wait for BOOT button release ─────────────────────────────
    // If GPIO0 is LOW during boot, ESP32 may enter download mode or have
    // wake issues. Wait for button release before continuing.
    pinMode(PIN_BOOT_BUTTON, INPUT);
    pinMode(PIN_LED, OUTPUT);
    
    // Blink LED rapidly while waiting for button release (visual feedback)
    while (digitalRead(PIN_BOOT_BUTTON) == LOW) {
        digitalWrite(PIN_LED, HIGH);
        delay(50);
        digitalWrite(PIN_LED, LOW);
        delay(50);
    }
    delay(100);  // Debounce after release
    // ─────────────────────────────────────────────────────────────────────────
    
    Serial.begin(115200);
    delay(500);
    
    Serial.println();
    Serial.println("========================================");
    Serial.println("FighterLink Boxing Glove Firmware");
    Serial.println("Board: ESP32 DevKit (Light Sleep Mode)");
    Serial.printf("Hand: %s\n", HAND_ID == 0 ? "LEFT" : "RIGHT");
    Serial.println("========================================");
    Serial.println();
    
    // Initialize BOOT button as input (has external pull-up on DevKit)
    pinMode(PIN_BOOT_BUTTON, INPUT);
    
    // Initialize LED
    pinMode(PIN_LED, OUTPUT);
    setLed(false);
    
    // Initialize MPU6050 (no blocking calibration)
    if (!setupMPU()) {
        Serial.println("FATAL: MPU6050 initialization failed!");
        Serial.println("Check wiring: SDA→GPIO21, SCL→GPIO22");
        // Rapid blink to indicate error
        while (true) {
            setLed(true);
            delay(100);
            setLed(false);
            delay(100);
        }
    }
    
    // Initialize BLE
    setupBLE();
    
    // Start in advertising state
    changeState(STATE_ADVERTISING);
    
    Serial.println();
    Serial.println("Ready! Looking for BLE connection...");
    Serial.printf("Timeout: %d seconds (then goes to idle)\n", ADVERTISING_TIMEOUT_MS / 1000);
    Serial.println("Press BOOT button to toggle between idle and advertising.");
    Serial.println("Hold glove STILL for 3 seconds to calibrate.");
}

// ═══════════════════════════════════════════════════════════════════════════════
// MAIN LOOP
// ═══════════════════════════════════════════════════════════════════════════════

void loop() {
    uint32_t now = millis();
    
    // ─── State Machine ───────────────────────────────────────────────────────
    switch (currentState) {
        
        // ─── IDLE STATE (Light Sleep) ────────────────────────────────────────
        case STATE_IDLE: {
            // Handle idle mode with light sleep
            // Button press is handled inside handleIdleState()
            handleIdleState();
            break;
        }
        
        // ─── ADVERTISING STATE ───────────────────────────────────────────────
        case STATE_ADVERTISING: {
            // Check for BOOT button - go to idle
            if (checkBootButtonPressed()) {
                Serial.println("BOOT button pressed - going to idle");
                enterIdleMode();
                break;
            }
            
            // Check for connection
            if (deviceConnected) {
                wasEverConnected = true;
                if (isCalibrated) {
                    Serial.println("Connected! Starting sensor streaming at 100Hz...");
                } else {
                    Serial.println("Connected (uncalibrated) - hold still to calibrate...");
                }
                changeState(STATE_CONNECTED);
                break;
            }
            
            // Check for timeout
            uint32_t elapsed = now - stateStartTime;
            if (elapsed >= ADVERTISING_TIMEOUT_MS) {
                Serial.println("Advertising timeout - no connection found");
                enterIdleMode();
                break;
            }
            
            // LED: Fast blink while advertising
            blinkLed(LED_BLINK_FAST_MS);
            
            // Smart calibration (runs while advertising)
            static uint32_t lastCalibCheckAdv = 0;
            if (now - lastCalibCheckAdv >= SAMPLE_RATE_MS) {
                updateSmartCalibration();
                lastCalibCheckAdv = now;
            }
            
            // Progress indicator every 10 seconds
            static uint32_t lastProgressPrint = 0;
            if (now - lastProgressPrint >= 10000) {
                uint32_t remaining = (ADVERTISING_TIMEOUT_MS - elapsed) / 1000;
                Serial.printf("Still advertising... %d seconds remaining\n", remaining);
                lastProgressPrint = now;
            }
            break;
        }
        
        // ─── CONNECTED STATE ─────────────────────────────────────────────────
        case STATE_CONNECTED: {
            // Check for BOOT button - go to idle
            if (checkBootButtonPressed()) {
                Serial.println("BOOT button pressed - disconnecting and going to idle");
                enterIdleMode();
                break;
            }
            
            // Check for disconnection
            if (!deviceConnected) {
                Serial.println("Connection lost!");
                Serial.printf("Attempting to reconnect for %d seconds...\n", RECONNECT_TIMEOUT_MS / 1000);
                delay(500);
                pServer->startAdvertising();
                changeState(STATE_RECONNECTING);
                break;
            }
            
            // Stream sensor data at 100Hz
            if (now - lastSampleTime >= SAMPLE_RATE_MS) {
                sendSensorData();
                lastSampleTime = now;
            }
            
            // Smart calibration (runs while connected)
            static uint32_t lastCalibCheckConn = 0;
            if (now - lastCalibCheckConn >= SAMPLE_RATE_MS) {
                updateSmartCalibration();
                lastCalibCheckConn = now;
            }
            
            // LED: Solid ON when calibrated, medium blink when not
            if (isCalibrated) {
                setLed(true);
            } else {
                blinkLed(LED_BLINK_CALIBRATING);
            }
            break;
        }
        
        // ─── RECONNECTING STATE ──────────────────────────────────────────────
        case STATE_RECONNECTING: {
            // Check for BOOT button - go to idle
            if (checkBootButtonPressed()) {
                Serial.println("BOOT button pressed - canceling reconnect, going to idle");
                enterIdleMode();
                break;
            }
            
            // Check for reconnection
            if (deviceConnected) {
                Serial.println("Reconnected!");
                changeState(STATE_CONNECTED);
                break;
            }
            
            // Check for timeout
            uint32_t elapsed = now - stateStartTime;
            if (elapsed >= RECONNECT_TIMEOUT_MS) {
                Serial.println("Reconnect timeout - giving up");
                enterIdleMode();
                break;
            }
            
            // LED: Slow blink while reconnecting (different from advertising)
            blinkLed(LED_BLINK_SLOW_MS);
            
            // Progress indicator every 10 seconds
            static uint32_t lastReconnectPrint = 0;
            if (now - lastReconnectPrint >= 10000) {
                uint32_t remaining = (RECONNECT_TIMEOUT_MS - elapsed) / 1000;
                Serial.printf("Reconnecting... %d seconds remaining\n", remaining);
                lastReconnectPrint = now;
            }
            break;
        }
        
        default:
            // Should never happen
            Serial.println("ERROR: Unknown state!");
            enterIdleMode();
            break;
    }
    
    // Update oldDeviceConnected for next iteration
    oldDeviceConnected = deviceConnected;
}
