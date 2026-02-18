/**
 * FighterLink Boxing Glove Firmware
 * 
 * BLE Peripheral that streams MPU6050 sensor data at 100Hz.
 * Automatically starts advertising on power-up.
 * 
 * Hardware: Seeed Studio XIAO ESP32C3 + MPU6050
 * 
 * LED States:
 *   - Slow blink (1s): Initializing / Calibrating
 *   - Fast blink (200ms): Advertising, waiting for connection
 *   - Solid ON: Connected, streaming data
 *   - Double blink: Session event acknowledgment
 */

#include <Arduino.h>
#include <Wire.h>
#include <BLEDevice.h>
#include <BLEServer.h>
#include <BLEUtils.h>
#include <BLE2902.h>
#include <MPU6050_light.h>

#include "config.h"
#include "sensor_packet.h"

// ─── Global Objects ──────────────────────────────────────────────────────────
MPU6050 mpu(Wire);

BLEServer* g_pServer = nullptr;
BLECharacteristic* g_pSensorChar = nullptr;
BLECharacteristic* g_pBatteryChar = nullptr;
BLECharacteristic* g_pDeviceChar = nullptr;

// ─── Global State ────────────────────────────────────────────────────────────
volatile bool g_deviceConnected = false;
volatile bool g_oldDeviceConnected = false;

uint16_t g_sequenceNumber = 0;
uint32_t g_lastSampleTime = 0;
uint32_t g_lastBatteryTime = 0;
uint32_t g_lastLedToggle = 0;
bool g_ledState = false;
bool g_isCalibrated = false;

// ─── BLE Callbacks ───────────────────────────────────────────────────────────
class ServerCallbacks : public BLEServerCallbacks {
    void onConnect(BLEServer* pServer) override {
        g_deviceConnected = true;
        Serial.println("BLE: Client connected");
    }

    void onDisconnect(BLEServer* pServer) override {
        g_deviceConnected = false;
        Serial.println("BLE: Client disconnected");
    }
};

// ─── LED Control ─────────────────────────────────────────────────────────────
void setLed(bool on) {
    // XIAO ESP32C3 onboard LED is active LOW
    digitalWrite(PIN_LED, on ? LOW : HIGH);
    g_ledState = on;
}

void blinkLed(uint32_t periodMs) {
    uint32_t now = millis();
    if (now - g_lastLedToggle >= periodMs / 2) {
        setLed(!g_ledState);
        g_lastLedToggle = now;
    }
}

void doubleBlink() {
    for (int i = 0; i < 2; i++) {
        setLed(true);
        delay(100);
        setLed(false);
        delay(100);
    }
}

// ─── Battery Monitoring ──────────────────────────────────────────────────────
uint8_t readBatteryLevel() {
    // Read ADC value from battery voltage pin
    // Note: XIAO ESP32C3 ADC is 12-bit (0-4095)
    // Adjust this based on your voltage divider circuit
    
    int adcValue = analogRead(PIN_VBAT);
    
    // Convert ADC to voltage (assuming 3.3V reference, voltage divider may vary)
    // This is a simplified calculation - calibrate for your specific circuit
    float voltage = (adcValue / 4095.0f) * 3.3f * 2.0f;  // *2 for voltage divider
    
    // Map voltage to percentage
    float percentage = (voltage - VBAT_MIN) / (VBAT_MAX - VBAT_MIN) * 100.0f;
    percentage = constrain(percentage, 0.0f, 100.0f);
    
    return (uint8_t)percentage;
}

bool isCharging() {
    // Check if charging voltage is present on pogo pins
    int adcValue = analogRead(PIN_VCHARGE);
    float voltage = (adcValue / 4095.0f) * 3.3f * 2.0f;
    return voltage > VCHARGE_THRESH;
}

// ─── BLE Setup ───────────────────────────────────────────────────────────────
void setupBLE() {
    Serial.println("BLE: Initializing...");
    
    // Initialize BLE with device name
    BLEDevice::init(BLE_DEVICE_NAME);
    
    // Create BLE Server
    g_pServer = BLEDevice::createServer();
    g_pServer->setCallbacks(new ServerCallbacks());
    
    // Create FighterLink Service
    BLEService* pService = g_pServer->createService(BLE_SERVICE_UUID);
    
    // Create Sensor Data Characteristic (NOTIFY only)
    g_pSensorChar = pService->createCharacteristic(
        BLE_CHAR_SENSOR_UUID,
        BLECharacteristic::PROPERTY_NOTIFY
    );
    g_pSensorChar->addDescriptor(new BLE2902());  // CCCD for notifications
    
    // Create Battery Level Characteristic (READ + NOTIFY)
    g_pBatteryChar = pService->createCharacteristic(
        BLE_CHAR_BATTERY_UUID,
        BLECharacteristic::PROPERTY_READ | BLECharacteristic::PROPERTY_NOTIFY
    );
    g_pBatteryChar->addDescriptor(new BLE2902());
    uint8_t initBattery = readBatteryLevel();
    g_pBatteryChar->setValue(&initBattery, 1);
    
    // Create Device Info Characteristic (READ only - returns hand ID)
    g_pDeviceChar = pService->createCharacteristic(
        BLE_CHAR_DEVICE_UUID,
        BLECharacteristic::PROPERTY_READ
    );
    uint8_t handId = HAND_ID;
    g_pDeviceChar->setValue(&handId, 1);
    
    // Start the service
    pService->start();
    
    // Start advertising
    BLEAdvertising* pAdvertising = BLEDevice::getAdvertising();
    pAdvertising->addServiceUUID(BLE_SERVICE_UUID);
    pAdvertising->setScanResponse(true);
    pAdvertising->setMinPreferred(0x06);  // Help with iPhone connections
    pAdvertising->setMinPreferred(0x12);
    BLEDevice::startAdvertising();
    
    Serial.printf("BLE: Advertising as '%s'\n", BLE_DEVICE_NAME);
}

// ─── MPU6050 Setup ───────────────────────────────────────────────────────────
bool setupMPU() {
    Serial.println("MPU6050: Initializing...");
    
    Wire.begin(PIN_SDA, PIN_SCL);
    
    byte status = mpu.begin();
    if (status != 0) {
        Serial.printf("MPU6050: Init failed with status %d\n", status);
        return false;
    }
    
    Serial.println("MPU6050: Ready. Calibrating - keep device still...");
    
    // Slow blink during calibration
    for (int i = 0; i < 6; i++) {  // ~3 seconds of blinking
        setLed(true);
        delay(250);
        setLed(false);
        delay(250);
    }
    
    // Calibrate offsets (accelerometer and gyroscope)
    mpu.calcOffsets(true, true);
    
    g_isCalibrated = true;
    Serial.println("MPU6050: Calibration complete");
    
    return true;
}

// ─── Send Sensor Data ────────────────────────────────────────────────────────
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
    packet.sequence = g_sequenceNumber++;
    
    // Battery level (cached, updated less frequently)
    static uint8_t cachedBattery = 100;
    packet.battery = cachedBattery;
    
    // Flags
    packet.flags = 0;
    if (isCharging()) {
        packet.flags |= FLAG_CHARGING;
    }
    if (g_isCalibrated) {
        packet.flags |= FLAG_CALIBRATED;
    }
    
    // Send via BLE notification
    g_pSensorChar->setValue((uint8_t*)&packet, sizeof(SensorPacket));
    g_pSensorChar->notify();
}

// ─── Update Battery Characteristic ───────────────────────────────────────────
void updateBattery() {
    uint8_t level = readBatteryLevel();
    g_pBatteryChar->setValue(&level, 1);
    g_pBatteryChar->notify();
    
    Serial.printf("Battery: %d%%\n", level);
}

// ─── Setup ───────────────────────────────────────────────────────────────────
void setup() {
    Serial.begin(115200);
    delay(1000);  // Wait for serial monitor
    
    Serial.println("\n========================================");
    Serial.println("FighterLink Boxing Glove Firmware");
    Serial.printf("Hand: %s\n", HAND_ID == 0 ? "LEFT" : "RIGHT");
    Serial.println("========================================\n");
    
    // Initialize LED
    pinMode(PIN_LED, OUTPUT);
    setLed(false);
    
    // Initialize battery monitoring pins
    pinMode(PIN_VBAT, INPUT);
    pinMode(PIN_VCHARGE, INPUT);
    
    // Check if in charging case
    if (isCharging()) {
        Serial.println("Charging detected - entering deep sleep");
        Serial.println("Remove from case to activate");
        // In a full implementation, we would enter deep sleep here
        // For now, just indicate charging state
        while (isCharging()) {
            setLed(true);
            delay(2000);
            setLed(false);
            delay(2000);
        }
    }
    
    // Initialize MPU6050
    if (!setupMPU()) {
        Serial.println("FATAL: MPU6050 initialization failed");
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
    
    // Quick triple blink to indicate ready
    for (int i = 0; i < 3; i++) {
        setLed(true);
        delay(100);
        setLed(false);
        delay(100);
    }
    
    Serial.println("Setup complete - waiting for BLE connection...");
}

// ─── Main Loop ───────────────────────────────────────────────────────────────
void loop() {
    uint32_t now = millis();
    
    // Handle connection state changes
    if (g_deviceConnected && !g_oldDeviceConnected) {
        // Just connected
        Serial.println("Starting sensor streaming...");
        setLed(true);  // Solid LED when connected
        g_oldDeviceConnected = true;
    }
    
    if (!g_deviceConnected && g_oldDeviceConnected) {
        // Just disconnected
        Serial.println("Connection lost - restarting advertising...");
        delay(500);  // Give BLE stack time to reset
        g_pServer->startAdvertising();
        g_oldDeviceConnected = false;
    }
    
    // When connected: stream sensor data at 100Hz
    if (g_deviceConnected) {
        if (now - g_lastSampleTime >= SAMPLE_RATE_MS) {
            sendSensorData();
            g_lastSampleTime = now;
        }
        
        // Update battery level periodically
        if (now - g_lastBatteryTime >= BATTERY_UPDATE_MS) {
            updateBattery();
            g_lastBatteryTime = now;
        }
    } else {
        // When not connected: fast blink to show advertising
        blinkLed(LED_BLINK_FAST_MS);
    }
}
