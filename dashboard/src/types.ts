// Types mirror the Go structs broadcast over WebSocket.

export interface PunchEvent {
  hand: string       // "left" | "right"
  type: string       // "straight" | "hook" | "uppercut" | "unknown"
  force: number      // m/s²
  rotation_z: number // peak °/s
  ts: number         // ESP32 millis
  count: number      // punch number in session
}

export interface HandState {
  connected: boolean
  calibrated: boolean
  battery: number
  packet_loss: number
  punch_count: number
  punch_breakdown: Record<string, number>
  max_force: number
  avg_force: number
  ppm: number
  recent_punches: PunchEvent[]
  current_accel: [number, number, number]  // X, Y, Z in m/s²
  current_gyro: [number, number, number]   // X, Y, Z in °/s
}

export interface CombinedStats {
  total_punches: number
  avg_force: number
  max_force: number
  ppm: number
  pps: number              // punches per second
  intensity_score: number  // gamified score
}

export interface SessionState {
  active: boolean
  elapsed_sec: number
  left: HandState
  right: HandState
  combined: CombinedStats
  paused: boolean
}

// App phase for UI routing
export type AppPhase = 'pre' | 'live' | 'post'

// Round configuration
export interface RoundConfig {
  durationSec: number  // round duration in seconds (default 180 = 3 min)
  currentRound: number
  roundStartTime: number  // timestamp when current round started
}

export const defaultHandState: HandState = {
  connected: false,
  calibrated: false,
  battery: 0,
  packet_loss: 0,
  punch_count: 0,
  punch_breakdown: {},
  max_force: 0,
  avg_force: 0,
  ppm: 0,
  recent_punches: [],
  current_accel: [0, 0, 0],
  current_gyro: [0, 0, 0],
}

export const defaultSession: SessionState = {
  active: false,
  elapsed_sec: 0,
  left: { ...defaultHandState },
  right: { ...defaultHandState },
  combined: { total_punches: 0, avg_force: 0, max_force: 0, ppm: 0, pps: 0, intensity_score: 0 },
  paused: false,
}

// Helper: estimate battery life remaining (rough estimate: ~2 hours at 100%)
export function estimateBatteryLife(batteryPercent: number): string {
  const hoursRemaining = (batteryPercent / 100) * 2  // assume 2 hours at 100%
  if (hoursRemaining < 1) {
    return `${Math.round(hoursRemaining * 60)} min`
  }
  return `${hoursRemaining.toFixed(1)} hrs`
}

// Helper: format time as MM:SS
export function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60).toString().padStart(2, '0')
  const s = Math.floor(seconds % 60).toString().padStart(2, '0')
  return `${m}:${s}`
}
