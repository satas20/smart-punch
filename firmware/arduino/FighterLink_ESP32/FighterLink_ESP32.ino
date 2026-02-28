/**
 * FighterLink Boxing Glove Firmware
 * 
 * Arduino IDE version for ESP32 DevKit
 * 
 * BLE Peripheral that streams MPU6050 sensor data at 100Hz.
 * PROTOTYPE MODE: Always-on, no sleep/idle mode for easier development.
 * 
 * Hardware: ESP32 DevKit + MPU6050 + TP4056 (LiPo charger)
 *   - MPU6050 SDA → GPIO21
 *   - MPU6050 SCL → GPIO22
 *   - Onboard LED → GPIO2
 * 
 * Setup:
 *   1. Install ESP32 board support in Arduino IDE
 *   2. Install MPU6050_light library
 *   3. Select Board: "ESP32 Dev Module"
 *   4. Set HAND_ID below (0 = Left, 1 = Right)
 *   5. Upload
 * 
 * Operation (Always-On Prototype Mode):
 *   - On power-up: Starts BLE advertising indefinitely
 *   - BLE stays active forever until a connection is made
 *   - On disconnect: Automatically restarts advertising (no timeout)
 *   - Press EN button to reset the board if needed
 * 
 * LED States:
 *   - Fast blink (200ms): Advertising, looking for connection
 *   - Slow blink (1s): Reconnecting after disconnect
 *   - Medium blink (500ms): Connected but not calibrated
 *   - Solid ON: Connected and calibrated, streaming data
 * 
 * Power Consumption:
 *   - Always active: ~20-30mA (no sleep mode)
 *   - Best used with USB power or TP4056 charger during development
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

// ═══════════════════════════════════════════════════════════════════════════════
// TIMING CONSTANTS
// ═══════════════════════════════════════════════════════════════════════════════

#define SAMPLE_RATE_MS          10      // 10ms = 100Hz
#define LED_BLINK_FAST_MS       200     // Fast blink (advertising)
#define LED_BLINK_SLOW_MS       1000    // Slow blink (reconnecting)
#define LED_BLINK_CALIBRATING   500     // Medium blink (calibrating)

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
// STATE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════════

void changeState(DeviceState newState) {
    const char* stateNames[] = {"ADVERTISING", "CONNECTED", "RECONNECTING"};
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
    // Initialize LED first for visual feedback
    pinMode(PIN_LED, OUTPUT);
    digitalWrite(PIN_LED, LOW);
    
    Serial.begin(115200);
    delay(500);
    
    Serial.println();
    Serial.println("========================================");
    Serial.println("FighterLink Boxing Glove Firmware");
    Serial.println("Board: ESP32 DevKit (Always-On Mode)");
    Serial.printf("Hand: %s\n", HAND_ID == 0 ? "LEFT" : "RIGHT");
    Serial.println("========================================");
    Serial.println();
    
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
    Serial.println("BLE will stay active until connected (no timeout).");
    Serial.println("Press EN button to reset if needed.");
    Serial.println("Hold glove STILL for 3 seconds to calibrate.");
}

// ═══════════════════════════════════════════════════════════════════════════════
// MAIN LOOP
// ═══════════════════════════════════════════════════════════════════════════════

void loop() {
    uint32_t now = millis();
    
    // ─── State Machine ───────────────────────────────────────────────────────
    switch (currentState) {
        
        // ─── ADVERTISING STATE ───────────────────────────────────────────────
        case STATE_ADVERTISING: {
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
            
            // LED: Fast blink while advertising
            blinkLed(LED_BLINK_FAST_MS);
            
            // Smart calibration (runs while advertising)
            static uint32_t lastCalibCheckAdv = 0;
            if (now - lastCalibCheckAdv >= SAMPLE_RATE_MS) {
                updateSmartCalibration();
                lastCalibCheckAdv = now;
            }
            
            // Progress indicator every 30 seconds (just to show it's alive)
            static uint32_t lastProgressPrint = 0;
            if (now - lastProgressPrint >= 30000) {
                Serial.println("Still advertising... waiting for connection");
                lastProgressPrint = now;
            }
            break;
        }
        
        // ─── CONNECTED STATE ─────────────────────────────────────────────────
        case STATE_CONNECTED: {
            // Check for disconnection
            if (!deviceConnected) {
                Serial.println("Connection lost! Restarting advertising...");
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
            // Check for reconnection
            if (deviceConnected) {
                Serial.println("Reconnected!");
                changeState(STATE_CONNECTED);
                break;
            }
            
            // LED: Slow blink while reconnecting (different from advertising)
            blinkLed(LED_BLINK_SLOW_MS);
            
            // Progress indicator every 30 seconds
            static uint32_t lastReconnectPrint = 0;
            if (now - lastReconnectPrint >= 30000) {
                Serial.println("Still waiting for reconnection...");
                lastReconnectPrint = now;
            }
            break;
        }
        
        default:
            // Should never happen - restart advertising
            Serial.println("ERROR: Unknown state! Restarting advertising...");
            changeState(STATE_ADVERTISING);
            break;
    }
    
    // Update oldDeviceConnected for next iteration
    oldDeviceConnected = deviceConnected;
}
