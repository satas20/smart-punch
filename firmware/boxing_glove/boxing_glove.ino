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
 *
 * LED states (onboard LED, GPIO 2):
 *   Slow blink  (500ms) → connecting to WiFi
 *   Fast blink  (100ms) → waiting for server ACK (ping phase)
 *   Solid ON            → paired with server, streaming
 */

#include <WiFi.h>
#include <WiFiUdp.h>
#include <Wire.h>
#include "MPU6050_light.h"

// ─── Configuration ────────────────────────────────────────────────────────────
#define WIFI_SSID      "LunaWifi"
#define WIFI_PASS      "albay2002"
#define SERVER_IP      "192.168.1.118"  // Your PC's IP on LunaWifi
#define UDP_PORT       4210
#define SAMPLE_RATE_MS 10               // 10ms = 100Hz
#define PING_INTERVAL  2000             // ms between pings until ACK received
#define LED_PIN        2                // Onboard LED

// ─── Globals ──────────────────────────────────────────────────────────────────
WiFiUDP  udp;
MPU6050  mpu(Wire);

IPAddress serverIP;
bool      serverFound   = false;
uint32_t  lastPing      = 0;
uint32_t  lastSample    = 0;
uint32_t  lastLedToggle = 0;

char incomingBuf[64];

// ─── Setup ────────────────────────────────────────────────────────────────────
void setup() {
  Serial.begin(115200);
  pinMode(LED_PIN, OUTPUT);
  digitalWrite(LED_PIN, LOW);

  // ── WiFi: slow blink while connecting ─────────────────────────────────────
  Serial.printf("\nConnecting to %s", WIFI_SSID);
  WiFi.begin(WIFI_SSID, WIFI_PASS);
  while (WiFi.status() != WL_CONNECTED) {
    digitalWrite(LED_PIN, HIGH); delay(500);
    digitalWrite(LED_PIN, LOW);  delay(500);
    Serial.print(".");
  }
  digitalWrite(LED_PIN, LOW);
  Serial.printf("\nWiFi connected. ESP32 IP: %s\n", WiFi.localIP().toString().c_str());

  // ── UDP ───────────────────────────────────────────────────────────────────
  udp.begin(UDP_PORT);
  serverIP.fromString(SERVER_IP);
  Serial.printf("Server target: %s:%d\n", SERVER_IP, UDP_PORT);

  // ── MPU6050 ───────────────────────────────────────────────────────────────
  Wire.begin(21, 22);
  byte status = mpu.begin();
  if (status != 0) {
    Serial.printf("MPU6050 init failed (status %d). Check wiring.\n", status);
    while (1) { delay(1000); }
  }
  Serial.println("MPU6050 ready. Calibrating — keep device still...");
  mpu.calcOffsets(true, true);
  Serial.println("Calibration done.");

  // 3 quick blinks → ready, entering ping phase
  for (int i = 0; i < 3; i++) {
    digitalWrite(LED_PIN, HIGH); delay(100);
    digitalWrite(LED_PIN, LOW);  delay(100);
  }

  Serial.printf("Pinging server at %s every %dms...\n", SERVER_IP, PING_INTERVAL);
}

// ─── Ping server (replaces broadcast) ────────────────────────────────────────
void sendPing() {
  udp.beginPacket(serverIP, UDP_PORT);
  udp.print("{\"type\":\"discover\"}");
  udp.endPacket();
  Serial.printf("Ping → %s:%d\n", SERVER_IP, UDP_PORT);
}

// ─── Check for incoming UDP commands ─────────────────────────────────────────
void checkIncoming() {
  int packetSize = udp.parsePacket();
  if (packetSize <= 0) return;

  int len = udp.read(incomingBuf, sizeof(incomingBuf) - 1);
  if (len <= 0) return;
  incomingBuf[len] = '\0';

  String msg = String(incomingBuf);

  // ACK from server → paired, go solid ON
  if (!serverFound && msg.indexOf("\"ack\"") != -1) {
    serverFound = true;
    Serial.println("Server ACK received — paired! LED solid ON. Streaming...");
    digitalWrite(LED_PIN, HIGH);  // solid on permanently
  }

  // Session reset
  if (msg.indexOf("\"session_reset\"") != -1) {
    Serial.println("Session reset received.");
    udp.beginPacket(serverIP, UDP_PORT);
    udp.print("{\"type\":\"session_reset_ack\"}");
    udp.endPacket();
    // 2 quick blinks, then back solid
    for (int i = 0; i < 2; i++) {
      digitalWrite(LED_PIN, LOW);  delay(80);
      digitalWrite(LED_PIN, HIGH); delay(80);
    }
  }

  // Session start
  if (msg.indexOf("\"session_start\"") != -1) {
    Serial.println("Session start received.");
    // 1 long pulse, then back solid
    digitalWrite(LED_PIN, LOW);  delay(400);
    digitalWrite(LED_PIN, HIGH);
  }
}

// ─── Stream one sensor sample ─────────────────────────────────────────────────
void streamSample() {
  mpu.update();

  float ax = mpu.getAccX() * 9.81f;  // g → m/s²
  float ay = mpu.getAccY() * 9.81f;
  float az = mpu.getAccZ() * 9.81f;
  uint32_t ts = millis();

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

  // Phase 1: Ping until server ACKs
  if (!serverFound) {
    // Fast blink — non-blocking
    if (now - lastLedToggle >= 100) {
      digitalWrite(LED_PIN, !digitalRead(LED_PIN));
      lastLedToggle = now;
    }
    // Send ping every PING_INTERVAL ms
    if (now - lastPing >= PING_INTERVAL) {
      sendPing();
      lastPing = now;
    }
    checkIncoming();
    return;
  }

  // Phase 2: Paired — stream data + listen for commands
  checkIncoming();

  if (now - lastSample >= SAMPLE_RATE_MS) {
    streamSample();
    lastSample = now;
  }
}
