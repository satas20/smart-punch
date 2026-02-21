/**
 * FighterLink Boxing Glove Firmware
 * 
 * Arduino IDE version for ESP32 DevKit
 * 
 * BLE Peripheral that streams MPU6050 sensor data at 100Hz.
 * Automatically starts advertising on power-up.
 * 
 * Hardware: ESP32 DevKit + MPU6050
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
 * LED States:
 *   - Slow blink (1s): Initializing / Calibrating
 *   - Fast blink (200ms): Advertising, waiting for connection
 *   - Solid ON: Connected, streaming data
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
#define LED_BLINK_SLOW_MS       1000    // Slow blink (initializing)
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
    Serial.begin(115200);
    delay(1000);
    
    Serial.println();
    Serial.println("========================================");
    Serial.println("FighterLink Boxing Glove Firmware");
    Serial.println("Board: ESP32 DevKit");
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
    
    Serial.println("Setup complete - waiting for calibration...");
    Serial.println("Hold the glove STILL for 3 seconds to calibrate.");
    Serial.println("LED will blink medium speed until calibrated.");
}

// ═══════════════════════════════════════════════════════════════════════════════
// MAIN LOOP
// ═══════════════════════════════════════════════════════════════════════════════

void loop() {
    uint32_t now = millis();
    
    // Always run smart calibration check (even when not connected)
    // This allows calibration before/during BLE connection
    static uint32_t lastCalibrationCheck = 0;
    if (now - lastCalibrationCheck >= SAMPLE_RATE_MS) {
        updateSmartCalibration();
        lastCalibrationCheck = now;
    }
    
    // Handle connection state changes
    if (deviceConnected && !oldDeviceConnected) {
        // Just connected
        if (isCalibrated) {
            Serial.println("Starting sensor streaming at 100Hz...");
        } else {
            Serial.println("Connected (uncalibrated) - hold still to calibrate...");
        }
        oldDeviceConnected = true;
    }
    
    if (!deviceConnected && oldDeviceConnected) {
        // Just disconnected
        Serial.println("Connection lost - restarting advertising...");
        delay(500);
        pServer->startAdvertising();
        oldDeviceConnected = false;
    }
    
    // When connected: stream sensor data at 100Hz
    if (deviceConnected) {
        if (now - lastSampleTime >= SAMPLE_RATE_MS) {
            sendSensorData();
            lastSampleTime = now;
        }
        
        // LED state depends on calibration
        if (isCalibrated) {
            setLed(true);  // Solid LED when connected AND calibrated
        } else {
            blinkLed(LED_BLINK_CALIBRATING);  // Medium blink waiting for calibration
        }
    } else {
        // When not connected
        if (isCalibrated) {
            blinkLed(LED_BLINK_FAST_MS);  // Fast blink - ready, waiting for connection
        } else {
            blinkLed(LED_BLINK_CALIBRATING);  // Medium blink - needs calibration
        }
    }
}
