/**
 * IoT Smart Boxing Analytics Device — ESP32 Firmware
 *
 * Role: Dumb sensor. Streams raw MPU6050 accelerometer data at ~100Hz
 *       via UDP to the Go analytics server. Zero analysis on device.
 *
 * Hardware:
 *   MPU6050  SDA → GPIO 21
 *   MPU6050  SCL → GPIO 22
 *   Power:   3.7V LiPo → TP4056 OUT+ → ESP32 VIN
 *
 * Library required: MPU6050_light (install via Arduino Library Manager)
 */

#include <WiFi.h>
#include <WiFiUdp.h>
#include <Wire.h>
#include "MPU6050_light.h"

// ─── Configuration ────────────────────────────────────────────────────────────
#define WIFI_SSID       "LunaWifi"
#define WIFI_PASS       "albay2002"
#define UDP_PORT        4210
#define SAMPLE_RATE_MS  10     // 10ms = 100Hz
#define DISCOVERY_MS    2000   // Broadcast every 2s until ACK
#define LED_PIN         2      // Onboard LED (GPIO2 on most ESP32 boards)

// ─── Globals ──────────────────────────────────────────────────────────────────
WiFiUDP   udp;
MPU6050   mpu(Wire);

IPAddress serverIP;
bool      serverFound    = false;
uint32_t  lastDiscovery  = 0;
uint32_t  lastSample     = 0;

// Incoming command buffer
char      incomingBuf[64];

// ─── Setup ────────────────────────────────────────────────────────────────────
void setup() {
  Serial.begin(115200);
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(LED_PIN, LOW);

  // Connect to WiFi
  Serial.printf("\nConnecting to %s", WIFI_SSID);
  WiFi.begin(WIFI_SSID, WIFI_PASS);
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  Serial.printf("\nWiFi connected. IP: %s\n", WiFi.localIP().toString().c_str());

  // Start UDP
  udp.begin(UDP_PORT);

  // Initialize I2C and MPU6050
  Wire.begin(21, 22);  // SDA=21, SCL=22
  byte status = mpu.begin();
  if (status != 0) {
    Serial.printf("MPU6050 init failed (status %d). Check wiring.\n", status);
    while (1) { delay(1000); }
  }
  Serial.println("MPU6050 ready. Calibrating gyro offsets...");

  // Calibrate: keep device still for ~3 seconds
  mpu.calcOffsets(true, true);
  Serial.println("Calibration done.");

  // Blink LED 3x to signal ready
  for (int i = 0; i < 3; i++) {
    digitalWrite(LED_PIN, HIGH); delay(100);
    digitalWrite(LED_PIN, LOW);  delay(100);
  }

  Serial.println("Broadcasting for server...");
}

// ─── Discovery ────────────────────────────────────────────────────────────────
void broadcastDiscover() {
  udp.beginPacket(IPAddress(255, 255, 255, 255), UDP_PORT);
  udp.print("{\"type\":\"discover\"}");
  udp.endPacket();
  Serial.println("Discovery broadcast sent.");
}

void checkIncoming() {
  int packetSize = udp.parsePacket();
  if (packetSize <= 0) return;

  int len = udp.read(incomingBuf, sizeof(incomingBuf) - 1);
  if (len <= 0) return;
  incomingBuf[len] = '\0';

  String msg = String(incomingBuf);

  // ACK from server → lock onto its IP
  if (!serverFound && msg.indexOf("\"ack\"") != -1) {
    serverIP    = udp.remoteIP();
    serverFound = true;
    Serial.printf("Server found at %s\n", serverIP.toString().c_str());
    // Solid LED to confirm
    digitalWrite(LED_PIN, HIGH); delay(500); digitalWrite(LED_PIN, LOW);
  }

  // Session reset command from server
  if (msg.indexOf("\"session_reset\"") != -1) {
    Serial.println("Session reset command received.");
    // Send ack back
    udp.beginPacket(serverIP, UDP_PORT);
    udp.print("{\"type\":\"session_reset_ack\"}");
    udp.endPacket();
    // 2 quick blinks
    for (int i = 0; i < 2; i++) {
      digitalWrite(LED_PIN, HIGH); delay(80);
      digitalWrite(LED_PIN, LOW);  delay(80);
    }
  }

  // Session start command from server
  if (msg.indexOf("\"session_start\"") != -1) {
    Serial.println("Session start command received.");
    // 1 long blink
    digitalWrite(LED_PIN, HIGH); delay(400); digitalWrite(LED_PIN, LOW);
  }
}

// ─── Stream Sample ────────────────────────────────────────────────────────────
void streamSample() {
  mpu.update();

  float ax = mpu.getAccX();  // m/s²  (MPU6050_light returns g by default,
  float ay = mpu.getAccY();  //        multiply by 9.81 to get m/s²)
  float az = mpu.getAccZ();

  // Convert g → m/s²
  ax *= 9.81f;
  ay *= 9.81f;
  az *= 9.81f;

  uint32_t ts = millis();

  // Build compact JSON packet
  char buf[96];
  snprintf(buf, sizeof(buf),
    "{\"type\":\"data\",\"ax\":%.2f,\"ay\":%.2f,\"az\":%.2f,\"ts\":%lu}",
    ax, ay, az, ts
  );

  udp.beginPacket(serverIP, UDP_PORT);
  udp.print(buf);
  udp.endPacket();
}

// ─── Loop ─────────────────────────────────────────────────────────────────────
void loop() {
  uint32_t now = millis();

  // Phase 1: Discovery (until server is found)
  if (!serverFound) {
    if (now - lastDiscovery >= DISCOVERY_MS) {
      broadcastDiscover();
      lastDiscovery = now;
    }
    checkIncoming();  // Wait for ACK
    return;           // Don't stream until server is found
  }

  // Phase 2: Streaming + incoming command handling
  checkIncoming();

  if (now - lastSample >= SAMPLE_RATE_MS) {
    streamSample();
    lastSample = now;
  }
}
