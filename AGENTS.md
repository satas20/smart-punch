# AGENTS.md - FighterLink Codebase Guide

This document provides guidance for AI coding agents working on the FighterLink project,
a dual BLE boxing analytics system with three components: Dashboard (React), Server (Go + BLE), and Firmware (ESP32C3).

## Project Overview

FighterLink is a smart boxing glove system that uses BLE (Bluetooth Low Energy) to stream sensor data from two gloves (left/right) to a central device for real-time punch analytics.

**Key Architecture Points:**
- **BLE Protocol**: Dual peripheral topology (two gloves connect to one central)
- **Binary Data**: 20-byte packed structs instead of JSON for efficiency
- **Punch Classification**: Straight, Hook, Uppercut detection using accelerometer + gyroscope
- **Per-Hand Analytics**: Separate statistics tracked for each hand

## Project Structure

```
smart-punch/
├── README.md                    # Project documentation
├── AGENTS.md                    # This file - AI agent guidelines
│
├── firmware/
│   ├── platformio.ini           # PlatformIO config (XIAO ESP32C3)
│   ├── src/
│   │   └── main.cpp             # PlatformIO firmware
│   ├── include/
│   │   ├── config.h             # BLE UUIDs, pins, constants
│   │   └── sensor_packet.h      # Binary packet struct definition
│   ├── lib/                     # Local libraries
│   └── arduino/                 # Arduino IDE versions
│       └── FighterLink_ESP32/
│           └── FighterLink_ESP32.ino  # ESP32 DevKit (Arduino IDE)
│
├── server/                      # Go backend (BLE Central + WebSocket)
│   ├── go.mod                   # Go module (includes bluetooth dep)
│   ├── main.go                  # Entry point, HTTP/WS server
│   ├── ble/
│   │   ├── central.go           # BLE adapter initialization
│   │   ├── scanner.go           # Device discovery & connection
│   │   └── packet.go            # Binary packet parsing
│   ├── analytics/
│   │   └── analyzer.go          # Punch detection & classification
│   └── static/                  # Embedded React build output
│
└── dashboard/                   # React 18 + TypeScript + Vite frontend
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

## Build Commands

### Firmware (Arduino IDE / ESP32 DevKit)

```bash
# For ESP32 DevKit boards using Arduino IDE:

# 1. Open: firmware/arduino/FighterLink_ESP32/FighterLink_ESP32.ino
# 2. Set HAND_ID at top of file (0 = Left, 1 = Right)
# 3. Select Board: Tools → Board → ESP32 Arduino → "ESP32 Dev Module"
# 4. Select Port: Tools → Port → (your COM port)
# 5. Click Upload

# Required Libraries (install via Library Manager):
#   - MPU6050_light by rfetick

# Wiring:
#   MPU6050 SDA → GPIO21
#   MPU6050 SCL → GPIO22
```

### Firmware (PlatformIO / XIAO ESP32C3)

```bash
# Working directory: smart-punch/firmware

# Build firmware
pio run

# Build and upload to connected ESP32C3
pio run -t upload

# Monitor serial output
pio device monitor -b 115200

# Clean build
pio run -t clean

# Full rebuild and upload
pio run -t clean && pio run -t upload
```

**Important:** Before flashing, edit `include/config.h` to set `HAND_ID`:
- `0` for Left Glove (`FighterLink_L`)
- `1` for Right Glove (`FighterLink_R`)

### Server (Go + BLE)

```bash
# Working directory: smart-punch/server

# Download dependencies (required first time)
go mod tidy

# Run development server
go run .

# Build production binary
go build -o fighterlink-server .

# Run the binary
./fighterlink-server

# Run with verbose BLE logging
DEBUG_BLE=1 go run .
```

**Linux BLE Permissions:**
```bash
# Grant BLE capabilities to Go binary (required once)
sudo setcap 'cap_net_raw,cap_net_admin=eip' ./fighterlink-server

# Or run with sudo during development
sudo go run .
```

### Dashboard (React/TypeScript)

```bash
# Working directory: smart-punch/dashboard

# Install dependencies
npm install

# Start development server (port 5173, proxies to Go server on 8080)
npm run dev

# Type-check and build for production (outputs to ../server/static)
npm run build

# Preview production build
npm run preview
```

## Testing

### Server (Go)

```bash
# Working directory: smart-punch/server

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test -v ./ble/...
go test -v ./analytics/...

# Run single test
go test -run TestPacketParsing ./ble/...

# Test with race detector
go test -race ./...
```

### Dashboard (Vitest - if configured)

```bash
npm install -D vitest @testing-library/react
npx vitest
npx vitest path/to/file.test.ts
```

## Code Style Guidelines

### C++ (Firmware)

**Naming Conventions:**
- Constants/Macros: `SCREAMING_SNAKE_CASE` (e.g., `BLE_SERVICE_UUID`, `SAMPLE_RATE_MS`)
- Functions: `camelCase` (e.g., `sendSensorData`, `initializeBLE`)
- Global variables: `g_` prefix + camelCase (e.g., `g_isConnected`, `g_lastSampleTime`)
- Structs: `PascalCase` (e.g., `SensorPacket`)
- Local variables: `camelCase`

**Code Organization:**
```cpp
// ─── Section Name ────────────────────────────────────────
// Use section dividers for logical groupings
```

**Include Order:**
```cpp
// 1. Arduino/ESP32 core headers
#include <Arduino.h>
#include <BLEDevice.h>

// 2. Third-party libraries
#include <MPU6050_light.h>

// 3. Local project headers
#include "config.h"
#include "sensor_packet.h"
```

### Go (Server)

**Imports:**
```go
import (
    // Standard library
    "encoding/binary"
    "log"
    "sync"
    
    // External dependencies
    "tinygo.org/x/bluetooth"
    
    // Local packages
    "boxing-analytics/ble"
    "boxing-analytics/analytics"
)
```

**Naming Conventions:**
- Exported types/functions: `PascalCase` (e.g., `ParsePacket`, `SensorPacket`)
- Unexported: `camelCase` (e.g., `handleNotification`, `connectToDevice`)
- Constants: `camelCase` for unexported, `PascalCase` for exported
- Interfaces: `PascalCase` with `-er` suffix if single method (e.g., `PacketHandler`)

**Error Handling:**
```go
if err != nil {
    log.Printf("operation failed: %v", err)
    return err  // or return fmt.Errorf("context: %w", err)
}
```

**Concurrency:**
```go
type SafeStruct struct {
    mu   sync.RWMutex
    data map[string]int
}

func (s *SafeStruct) Get(key string) int {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.data[key]
}
```

### TypeScript/React (Dashboard)

**Imports:**
```typescript
// 1. External libraries first
import { useState, useEffect } from 'react'
import { LineChart, Line } from 'recharts'

// 2. Local imports second
import { useBoxingSocket } from './hooks/useBoxingSocket'
import type { SessionState, PunchEvent } from './types'
```

**Naming Conventions:**
- Components: `PascalCase` (e.g., `PunchCounter`, `ForceChart`)
- Hooks: `camelCase` with `use` prefix (e.g., `useBoxingSocket`)
- Interfaces/Types: `PascalCase` (e.g., `SessionState`, `HandState`)
- Variables/functions: `camelCase`

## Architecture Notes

### BLE Communication

```
┌─────────────────┐     ┌─────────────────┐
│ FighterLink_L   │     │ FighterLink_R   │
│ (Peripheral)    │     │ (Peripheral)    │
└────────┬────────┘     └────────┬────────┘
         │ GATT NOTIFY           │ GATT NOTIFY
         │ 20 bytes @ 100Hz      │ 20 bytes @ 100Hz
         ▼                       ▼
┌─────────────────────────────────────────┐
│         Go Server (Central)             │
│  tinygo.org/x/bluetooth                 │
│  Maintains 2 simultaneous connections   │
└─────────────────────────────────────────┘
```

### Binary Packet Structure (20 bytes)

```
Offset | Size | Type   | Field     | Scale
-------|------|--------|-----------|-------
0      | 2    | int16  | accX      | ÷100 → m/s²
2      | 2    | int16  | accY      | ÷100 → m/s²
4      | 2    | int16  | accZ      | ÷100 → m/s²
6      | 2    | int16  | gyroX     | ÷10 → °/s
8      | 2    | int16  | gyroY     | ÷10 → °/s
10     | 2    | int16  | gyroZ     | ÷10 → °/s
12     | 4    | uint32 | timestamp | ms
16     | 2    | uint16 | sequence  | packet counter
18     | 1    | uint8  | battery   | 0-100%
19     | 1    | uint8  | flags     | bit flags
```

### WebSocket Data Flow

```
BLE Notification → ble/packet.go (parse)
                 → analytics/analyzer.go (detect punch)
                 → main.go Hub (broadcast)
                 → WebSocket clients
```

## Dependencies

### Firmware
- **Board**: Seeed Studio XIAO ESP32C3
- **Framework**: Arduino
- **Libraries**:
  - `MPU6050_light` - Sensor library
  - ESP32 BLE (built into ESP32 Arduino core)

### Server
- **Go**: 1.21+
- **Dependencies**:
  - `tinygo.org/x/bluetooth` - BLE Central support

### Dashboard
- `react`, `react-dom`: ^18.3.1
- `recharts`: ^2.12.7
- Dev: TypeScript 5.4.5, Vite 5.3.1

## Common Tasks

### Adding a New Punch Type

1. Update classification logic in `server/analytics/analyzer.go`
2. Add new type to `PunchType` enum/constants
3. Update `HandState.PunchBreakdown` map handling
4. Update dashboard components to display new type

### Modifying Sensor Packet

1. Update `firmware/include/sensor_packet.h` struct
2. Update `server/ble/packet.go` parsing logic
3. Ensure byte alignment and total size match
4. Update documentation in README.md

### Adding a New BLE Characteristic

1. Add UUID to `firmware/include/config.h`
2. Create characteristic in `firmware/src/main.cpp`
3. Add discovery logic in `server/ble/scanner.go`
4. Handle reads/notifications in `server/ble/central.go`

### Adjusting Punch Detection Parameters

Edit `server/analytics/analyzer.go`:
```go
const (
    punchThreshold      = 35.0  // m/s² - minimum acceleration
    debounceMS          = 300   // minimum ms between punches
    hookRotationThresh  = 200.0 // °/s - gyro Z for hook detection
    upperRotationThresh = 150.0 // °/s - gyro X for uppercut
)
```

## Debugging

### BLE Issues (Linux)

```bash
# Check Bluetooth adapter
hciconfig -a

# Reset adapter
sudo hciconfig hci0 reset

# Monitor BLE traffic
sudo btmon

# List BLE devices
bluetoothctl
> scan on
> devices
```

### Firmware Debug

```bash
# Serial monitor with timestamps
pio device monitor -b 115200 --filter time

# View BLE advertisements (requires nRF Connect or similar)
```

### Server Debug

```bash
# Enable verbose logging
DEBUG_BLE=1 go run .

# Check for race conditions
go run -race .
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `:8080` | HTTP/WebSocket server port |
| `DEBUG_BLE` | `false` | Enable verbose BLE logging |
| `PUNCH_THRESHOLD` | `35.0` | Punch detection threshold (m/s²) |

## File Descriptions

| File | Purpose |
|------|---------|
| `firmware/platformio.ini` | PlatformIO project config for ESP32C3 |
| `firmware/include/config.h` | BLE UUIDs, pin mappings, constants |
| `firmware/include/sensor_packet.h` | Binary packet struct definition |
| `firmware/src/main.cpp` | Main firmware: BLE server + sensor reading |
| `server/main.go` | Entry point, HTTP server, WebSocket hub |
| `server/ble/central.go` | BLE adapter initialization |
| `server/ble/scanner.go` | Device discovery and connection |
| `server/ble/packet.go` | Binary packet parsing |
| `server/analytics/analyzer.go` | Punch detection and classification |
