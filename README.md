# IoT Smart Boxing Analytics Device

A wearable, low-cost IoT system that quantifies combat sports performance in real-time. The ESP32 streams raw accelerometer data continuously to a Go server, which performs all analysis and pushes live metrics to a React dashboard over WebSocket.

---

## Architecture

```
┌─────────────────────────────────────────────┐
│           ESP32 + MPU6050 (Glove)           │
│                                             │
│  MPU6050 ──I2C──► raw ax, ay, az @ 100Hz   │
│                        │                   │
│              UDP packet every 10ms          │
│              {"type":"data","ax":...}        │
└────────────────────────┬────────────────────┘
                         │  UDP :4210
                         │  (LunaWifi)
                         ▼
┌─────────────────────────────────────────────┐
│              Go Server (:8080)              │
│                                             │
│  UDP Listener                               │
│    ├─ Discovery handshake (broadcast ACK)   │
│    └─ Raw sample processing @ 100Hz         │
│                                             │
│  Analytics Engine (per packet)              │
│    ├─ magnitude = √(ax²+ay²+az²)            │
│    ├─ Punch detection (threshold 35 m/s²)   │
│    ├─ Debounce (300ms)                      │
│    ├─ Rolling 5s buffer (500 samples)       │
│    └─ Stats: avg force, max, PPM            │
│                                             │
│  WebSocket Hub (/ws)                        │
│    └─ Broadcasts SessionState on each punch │
│                                             │
│  REST API                                   │
│    ├─ POST /api/session/start               │
│    └─ POST /api/session/reset               │
└────────────────────────┬────────────────────┘
                         │  WebSocket ws://localhost:8080/ws
                         ▼
┌─────────────────────────────────────────────┐
│         React Dashboard (:5173 dev)         │
│                                             │
│  PunchCounter  — live punch total           │
│  ForceChart    — m/s² per punch (last 50)   │
│  StatsBar      — avg / max / PPM / timer    │
│  START / RESET — session control            │
└─────────────────────────────────────────────┘
```

---

## Project Structure

```
.
├── firmware/
│   └── main.ino          # ESP32 Arduino sketch (MPU6050_light)
├── server/
│   ├── main.go           # Go server: UDP + WebSocket + HTTP
│   ├── go.mod
│   └── static/           # React production build (git-ignored, Go-embedded)
└── dashboard/
    ├── index.html
    ├── package.json
    ├── vite.config.ts
    ├── tsconfig.json
    └── src/
        ├── main.tsx
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

## Hardware

| Component | Detail |
|---|---|
| Microcontroller | ESP32 (dual-core, Wi-Fi) |
| Sensor | GY-521 / MPU6050 (6-axis accel + gyro) |
| SDA | GPIO 21 |
| SCL | GPIO 22 |
| Battery | 3.7V LiPo 1500mAh |
| Charger | TP4056 (Type-C) → ESP32 VIN |

---

## Algorithm

```
magnitude = √(ax² + ay² + az²)          // m/s², computed server-side

if magnitude > 35 m/s²
   AND time_since_last_punch > 300ms:
     → punch detected
     → update count, avg, max, PPM
     → broadcast to dashboard
```

The ESP32 streams **raw sensor data only**. All analysis runs in Go.

---

## Getting Started

### 1. Flash the ESP32

- Install **Arduino IDE** with ESP32 board support
- Install library: `MPU6050_light` (Library Manager)
- Open `firmware/main.ino`
- Set board to **ESP32 Dev Module**
- Flash

The device will:
1. Connect to `LunaWifi`
2. Broadcast UDP on `255.255.255.255:4210` until the Go server replies
3. Stream raw accelerometer data at 100Hz

### 2. Start the Go Server

```bash
cd server
go run main.go
# UDP listening on :4210
# HTTP/WS on :8080
```

### 3. Start the React Dashboard (development)

```bash
cd dashboard
npm install
npm run dev
# Open http://localhost:5173
```

Vite proxies `/ws` and `/api` to the Go server automatically.

### 4. Production Build (optional)

Builds React into `server/static/`, which Go embeds into the binary:

```bash
cd dashboard
npm run build

cd ../server
go build -o boxing-server .
./boxing-server
# Everything served on :8080
```

---

## Session Flow

```
Dashboard                    Go Server                 ESP32
    │                            │                        │
    │── POST /api/session/start ─►│                        │
    │                            │── UDP session_start ──►│
    │                            │                        │
    │                            │◄── UDP data (100Hz) ───│
    │                            │  (analysis runs here)  │
    │◄── WebSocket broadcast ────│                        │
    │   (punch events, stats)    │                        │
    │                            │                        │
    │── POST /api/session/reset ─►│                        │
    │                            │── UDP session_reset ──►│
    │◄── WebSocket broadcast ────│  (clear state)         │
```

---

## Key Numbers

| Parameter | Value |
|---|---|
| Sample rate | 100 Hz (10ms/packet) |
| UDP packet size | ~70 bytes JSON |
| Network load | ~7 KB/s |
| Rolling buffer | 500 samples (5 seconds) |
| Punch threshold | 35 m/s² (~3.6g) |
| Debounce window | 300 ms |
| Chart history | last 50 punches |
| Session storage | In-memory only |

---

## Roadmap

- [ ] BLE mode (no Wi-Fi required)
- [ ] 3D-printed glove mount enclosure
- [ ] Combo detection (rapid punch sequence recognition)
- [ ] Session history persistence (JSON / SQLite)
- [ ] Pre-movement / windup analysis using rolling buffer
- [ ] Mobile app (React Native)
