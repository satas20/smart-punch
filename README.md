# FighterLink - Dual BLE Boxing Analytics System

A wearable smart boxing glove system that quantifies combat sports performance in real-time. Two sensor-equipped gloves (Left/Right) connect directly to a PC or mobile device via Bluetooth Low Energy (BLE), enabling punch detection, force measurement, and punch type classification.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FighterLink System                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────┐           ┌─────────────────────┐                 │
│  │   FighterLink_L     │           │   FighterLink_R     │                 │
│  │   (Left Glove)      │           │   (Right Glove)     │                 │
│  │                     │           │                     │                 │
│  │  XIAO ESP32C3       │           │  XIAO ESP32C3       │                 │
│  │  + MPU6050          │           │  + MPU6050          │                 │
│  │  + LiPo 100-150mAh  │           │  + LiPo 100-150mAh  │                 │
│  │                     │           │                     │                 │
│  │  BLE Peripheral     │           │  BLE Peripheral     │                 │
│  └──────────┬──────────┘           └──────────┬──────────┘                 │
│             │                                  │                            │
│             │  BLE GATT NOTIFY                 │  BLE GATT NOTIFY          │
│             │  (20-byte binary packets)        │  (20-byte binary packets) │
│             │  @ 100Hz                         │  @ 100Hz                  │
│             ▼                                  ▼                            │
│  ┌──────────────────────────────────────────────────────────┐              │
│  │              Go Backend (BLE Central)                    │              │
│  │                                                          │              │
│  │  • Dual BLE connection management                        │              │
│  │  • Binary packet parsing                                 │              │
│  │  • Punch detection (threshold + debounce)                │              │
│  │  • Punch classification (straight/hook/uppercut)         │              │
│  │  • Per-hand analytics (force, count, PPM)                │              │
│  │  • WebSocket broadcast to UI                             │              │
│  └──────────────────────────────────────────────────────────┘              │
│                              │                                              │
│                              │ WebSocket                                    │
│                              ▼                                              │
│  ┌──────────────────────────────────────────────────────────┐              │
│  │              React Dashboard / Flutter App               │              │
│  │                                                          │              │
│  │  • Real-time punch counter (per hand)                    │              │
│  │  • Force visualization charts                            │              │
│  │  • Punch type breakdown                                  │              │
│  │  • Session statistics                                    │              │
│  └──────────────────────────────────────────────────────────┘              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## System Topology

**Protocol:** Bluetooth Low Energy (BLE) 5.0  
**Topology:** Star Topology  
**Central Node:** PC (Go Backend) or Smartphone (Flutter App)  
**Peripheral Nodes:** Left Glove (`FighterLink_L`) and Right Glove (`FighterLink_R`)

The Central device scans for and maintains **two simultaneous BLE connections** to receive real-time data from both hands.

---

## Hardware Stack

### Per Glove Unit

| Component | Specification |
|-----------|---------------|
| Microcontroller | Seeed Studio XIAO ESP32C3 |
| Sensor | MPU6050 (6-axis: Accelerometer + Gyroscope) |
| Battery | LiPo 3.7V, 100-150mAh |
| I2C Pins | SDA: GPIO6, SCL: GPIO7 |
| Status LED | GPIO10 (onboard) |

### Charging Case (Passive "Dumb" Case)

The charging case contains **NO microcontroller or logic**. It is purely for power delivery.

| Component | Specification |
|-----------|---------------|
| Battery | Large LiPo (2000-3000mAh) |
| Charging Modules | 2x TP4056 (one per glove slot) |
| Connectors | Pogo Pins (5V + GND per slot) |
| Input | USB-C for case charging |

**Power State Detection (Glove-Side Logic):**

The gloves detect their state by monitoring the charging voltage on a GPIO pin:

```
┌─────────────────────────────────────────────────────────────┐
│                    Power State Machine                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  V_CHARGE > 4.0V (Inside Case)                              │
│    → Stop BLE advertising                                   │
│    → Enter deep sleep                                       │
│    → Battery charges via TP4056                             │
│                                                             │
│  V_CHARGE == 0V (Removed from Case)                         │
│    → Wake from deep sleep                                   │
│    → Initialize MPU6050                                     │
│    → Calibrate sensors                                      │
│    → Start BLE advertising                                  │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## BLE Protocol

### Service & Characteristics

```
FighterLink Service
UUID: 00001234-0000-1000-8000-00805f9b34fb

├── Sensor Data Characteristic (NOTIFY)
│   UUID: 00001235-0000-1000-8000-00805f9b34fb
│   Value: 20-byte binary packet @ 100Hz
│
├── Battery Level Characteristic (READ, NOTIFY)
│   UUID: 00001236-0000-1000-8000-00805f9b34fb  
│   Value: uint8_t (0-100%)
│
└── Device Info Characteristic (READ)
    UUID: 00001237-0000-1000-8000-00805f9b34fb
    Value: uint8_t (0 = Left, 1 = Right)
```

### Device Names
- Left Glove: `FighterLink_L`
- Right Glove: `FighterLink_R`

---

## Binary Packet Protocol (20 Bytes)

To maximize BLE bandwidth efficiency, we use a packed binary struct instead of JSON.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    Sensor Packet (20 bytes)                             │
├────────┬────────┬────────┬────────┬────────┬────────┬──────────────────┤
│ Offset │ Size   │ Type   │ Name   │ Scale  │ Units  │ Description      │
├────────┼────────┼────────┼────────┼────────┼────────┼──────────────────┤
│ 0      │ 2      │ int16  │ accX   │ ÷100   │ m/s²   │ Accelerometer X  │
│ 2      │ 2      │ int16  │ accY   │ ÷100   │ m/s²   │ Accelerometer Y  │
│ 4      │ 2      │ int16  │ accZ   │ ÷100   │ m/s²   │ Accelerometer Z  │
│ 6      │ 2      │ int16  │ gyroX  │ ÷10    │ °/s    │ Gyroscope X      │
│ 8      │ 2      │ int16  │ gyroY  │ ÷10    │ °/s    │ Gyroscope Y      │
│ 10     │ 2      │ int16  │ gyroZ  │ ÷10    │ °/s    │ Gyroscope Z      │
│ 12     │ 4      │ uint32 │ ts     │ -      │ ms     │ Timestamp        │
│ 16     │ 2      │ uint16 │ seq    │ -      │ -      │ Sequence number  │
│ 18     │ 1      │ uint8  │ batt   │ -      │ %      │ Battery level    │
│ 19     │ 1      │ uint8  │ flags  │ -      │ -      │ Status flags     │
└────────┴────────┴────────┴────────┴────────┴────────┴──────────────────┘

Flags Byte (bit field):
  Bit 0: isCharging (1 = charging, 0 = on battery)
  Bit 1: isCalibrated (1 = calibration complete)
  Bit 2-7: Reserved for future use
```

### C Struct Definition (Firmware)

```c
struct __attribute__((packed)) SensorPacket {
    int16_t  accX;      // Accelerometer X * 100
    int16_t  accY;      // Accelerometer Y * 100
    int16_t  accZ;      // Accelerometer Z * 100
    int16_t  gyroX;     // Gyroscope X * 10
    int16_t  gyroY;     // Gyroscope Y * 10
    int16_t  gyroZ;     // Gyroscope Z * 10
    uint32_t timestamp; // millis()
    uint16_t sequence;  // Packet sequence number
    uint8_t  battery;   // Battery percentage (0-100)
    uint8_t  flags;     // Status flags
};
```

### Go Struct Definition (Backend)

```go
type SensorPacket struct {
    AccX      int16  // ÷100 = m/s²
    AccY      int16
    AccZ      int16
    GyroX     int16  // ÷10 = °/s
    GyroY     int16
    GyroZ     int16
    Timestamp uint32 // milliseconds
    Sequence  uint16 // packet sequence
    Battery   uint8  // percentage
    Flags     uint8  // status flags
}
```

---

## Punch Detection Algorithm

```
┌─────────────────────────────────────────────────────────────┐
│                  Punch Detection Pipeline                   │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  1. MAGNITUDE CALCULATION                                   │
│     magnitude = √(accX² + accY² + accZ²)                    │
│                                                             │
│  2. THRESHOLD CHECK                                         │
│     if magnitude > 35 m/s² (~3.6g)                          │
│        → Punch candidate detected                           │
│                                                             │
│  3. DEBOUNCE                                                │
│     if time_since_last_punch > 300ms                        │
│        → Confirmed punch                                    │
│                                                             │
│  4. CLASSIFICATION (using gyroscope data)                   │
│     │                                                       │
│     ├─ Low rotation (< 150°/s) + Forward accel              │
│     │  → STRAIGHT (Jab/Cross)                               │
│     │                                                       │
│     ├─ High Z-rotation (> 200°/s) + Lateral accel           │
│     │  → HOOK                                               │
│     │                                                       │
│     └─ High X-rotation (> 150°/s) + Upward accel            │
│        → UPPERCUT                                           │
│                                                             │
│  5. STATISTICS UPDATE                                       │
│     → Increment punch count (per hand)                      │
│     → Update max/avg force                                  │
│     → Calculate punches per minute                          │
│     → Broadcast to WebSocket clients                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Project Structure

```
smart-punch/
├── README.md                    # This file
├── AGENTS.md                    # AI coding agent guidelines
│
├── firmware/
│   ├── platformio.ini           # PlatformIO config (XIAO ESP32C3)
│   ├── src/
│   │   └── main.cpp             # PlatformIO firmware
│   ├── include/
│   │   ├── config.h             # BLE UUIDs, pins, constants
│   │   └── sensor_packet.h      # Binary packet struct
│   └── arduino/                 # Arduino IDE versions
│       └── FighterLink_ESP32/
│           └── FighterLink_ESP32.ino  # ESP32 DevKit (Arduino IDE)
│
├── server/                      # Go backend (BLE Central)
│   ├── go.mod                   # Go module definition
│   ├── main.go                  # Entry point, HTTP/WebSocket server
│   ├── ble/
│   │   ├── central.go           # BLE adapter management
│   │   ├── scanner.go           # Device discovery
│   │   └── packet.go            # Binary packet parsing
│   ├── analytics/
│   │   └── analyzer.go          # Punch detection & classification
│   └── static/                  # Embedded React build
│
└── dashboard/                   # React frontend (unchanged)
    ├── package.json
    ├── vite.config.ts
    └── src/
        ├── App.tsx
        ├── types.ts
        ├── hooks/
        │   └── useBoxingSocket.ts
        └── components/
            ├── PunchCounter.tsx
            ├── ForceChart.tsx
            └── StatsBar.tsx
```

---

## Getting Started

### Prerequisites

- **Arduino IDE 2.x** or **PlatformIO** (for firmware upload)
- **Go 1.21+**
- **Node.js 18+** (for dashboard development)
- **Bluetooth adapter** on PC (for BLE Central)

### 1. Flash the Firmware (Both Gloves)

Choose **Option A** (Arduino IDE) or **Option B** (PlatformIO) based on your setup.

#### Option A: Arduino IDE (ESP32 DevKit)

**Step 1: Install ESP32 Board Support**
1. Open Arduino IDE
2. Go to: **File → Preferences**
3. Add to "Additional Board Manager URLs":
   ```
   https://raw.githubusercontent.com/espressif/arduino-esp32/gh-pages/package_esp32_index.json
   ```
4. Go to: **Tools → Board → Board Manager**
5. Search "ESP32" and install **"esp32 by Espressif Systems"**

**Step 2: Install MPU6050 Library**
1. Go to: **Sketch → Include Library → Manage Libraries**
2. Search "MPU6050_light"
3. Install **"MPU6050_light by rfetick"**

**Step 3: Upload Firmware**
1. Open: `firmware/arduino/FighterLink_ESP32/FighterLink_ESP32.ino`
2. **Edit `HAND_ID`** at line 28:
   - Set to `0` for **Left Glove**
   - Set to `1` for **Right Glove**
3. Select: **Tools → Board → ESP32 Arduino → "ESP32 Dev Module"**
4. Select: **Tools → Port → (your COM port)**
5. Click **Upload** button
6. Repeat for the second glove with the other `HAND_ID`

**Wiring for ESP32 DevKit:**
```
MPU6050 SDA → GPIO21
MPU6050 SCL → GPIO22
MPU6050 VCC → 3.3V
MPU6050 GND → GND
```

#### Option B: PlatformIO (XIAO ESP32C3)

```bash
cd firmware

# For Left Glove: Set HAND_ID to 0 in include/config.h, then:
pio run -t upload

# For Right Glove: Set HAND_ID to 1 in include/config.h, then:
pio run -t upload
```

#### After Upload

The gloves will:
1. Initialize MPU6050 and calibrate (keep still for ~3 seconds)
2. Start BLE advertising as `FighterLink_L` or `FighterLink_R`
3. Fast-blink LED while waiting for connection
4. Solid LED when connected, streaming at 100Hz

### 2. Start the Go Backend

```bash
cd server
go mod tidy
go run .

# Output:
# BLE adapter enabled
# Scanning for FighterLink devices...
# Connected to FighterLink_L
# Connected to FighterLink_R
# HTTP/WS server on :8080
```

The backend will:
1. Initialize the BLE adapter
2. Scan for `FighterLink_L` and `FighterLink_R`
3. Connect to both gloves automatically
4. Start receiving and processing sensor data
5. Broadcast analytics via WebSocket

### 3. Start the Dashboard (Development)

```bash
cd dashboard
npm install
npm run dev
# Open http://localhost:5173
```

### 4. Production Build

```bash
# Build React dashboard
cd dashboard
npm run build

# Build Go server with embedded static files
cd ../server
go build -o fighterlink-server .
./fighterlink-server
# Everything served on :8080
```

---

## WebSocket API

### Endpoint
`ws://localhost:8080/ws`

### Message Format (Server → Client)

```json
{
  "active": true,
  "elapsed_sec": 125.5,
  "left": {
    "connected": true,
    "battery": 85,
    "packet_loss": 0.5,
    "punch_count": 45,
    "punch_breakdown": {
      "straight": 25,
      "hook": 15,
      "uppercut": 5
    },
    "max_force": 52.3,
    "avg_force": 38.7,
    "ppm": 21.6,
    "recent_punches": [
      {"hand": "left", "type": "hook", "force": 45.2, "ts": 123456, "count": 45}
    ]
  },
  "right": {
    "connected": true,
    "battery": 78,
    "packet_loss": 0.2,
    "punch_count": 52,
    "punch_breakdown": {
      "straight": 30,
      "hook": 18,
      "uppercut": 4
    },
    "max_force": 58.1,
    "avg_force": 41.2,
    "ppm": 24.9,
    "recent_punches": []
  },
  "combined": {
    "total_punches": 97,
    "avg_force": 40.1,
    "max_force": 58.1,
    "ppm": 46.5
  }
}
```

### REST API

| Endpoint | Method | Description |
|----------|--------|-------------|
| `POST /api/session/start` | POST | Start a new training session |
| `POST /api/session/reset` | POST | Reset session statistics |

---

## Key Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| Sample rate | 100 Hz | Sensor reading frequency |
| Packet size | 20 bytes | Binary BLE notification |
| BLE MTU | 23+ bytes | Minimum required MTU |
| Punch threshold | 35 m/s² | ~3.6g acceleration |
| Debounce window | 300 ms | Minimum time between punches |
| Chart history | 50 punches | Per-hand recent punch buffer |

---

## Sensor Mounting Options

The punch classification algorithm depends on sensor orientation. Document your chosen mounting:

### Option A: Top of Hand (Knuckle Area)
```
Sensor Orientation:
  +X = Punch direction (forward)
  +Y = Lateral (thumb side)
  +Z = Vertical (up from back of hand)
```

### Option B: Back of Wrist
```
Sensor Orientation:
  +X = Lateral
  +Y = Punch direction (forward)
  +Z = Vertical (up from wrist)
```

**Note:** Update the classification algorithm in `server/analytics/analyzer.go` based on your chosen mounting orientation.

---

## LED Status Indicators

| Pattern | Meaning |
|---------|---------|
| Slow blink (1s) | Initializing / Calibrating |
| Fast blink (200ms) | BLE Advertising, waiting for connection |
| Solid ON | Connected to Central, streaming data |
| Double blink | Session started |
| Off | Deep sleep (in charging case) |

---

## Roadmap

- [x] BLE dual-glove architecture
- [x] Binary packet protocol
- [x] Punch type classification
- [ ] Flutter mobile app (BLE Central)
- [ ] Charging case hardware design
- [ ] Deep sleep / wake-on-removal implementation
- [ ] Combo detection (rapid punch sequences)
- [ ] Session history persistence (SQLite)
- [ ] 3D-printed glove mount enclosure
- [ ] OTA firmware updates via BLE

---

## Troubleshooting

### BLE Connection Issues

1. **Glove not appearing in scan:**
   - Ensure glove is powered on and not in charging case
   - Check that BLE advertising has started (fast LED blink)
   - Restart the Go backend

2. **Frequent disconnections:**
   - Move closer to reduce interference
   - Check battery level
   - Ensure no other device is connected to the glove

3. **Missing packets / High packet loss:**
   - Reduce distance between glove and Central
   - Check for WiFi interference on 2.4GHz band
   - Monitor sequence numbers in logs

### Sensor Issues

1. **Erratic readings after startup:**
   - Keep glove still during calibration (3 seconds)
   - Recalibrate by power cycling the glove

2. **Punches not detected:**
   - Verify sensor is securely mounted
   - Check punch threshold in analytics config
   - Review accelerometer data in debug logs

---

## License

MIT License - See LICENSE file for details.
