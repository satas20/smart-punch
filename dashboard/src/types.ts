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
  battery: number
  packet_loss: number
  punch_count: number
  punch_breakdown: Record<string, number>
  max_force: number
  avg_force: number
  ppm: number
  recent_punches: PunchEvent[]
}

export interface CombinedStats {
  total_punches: number
  avg_force: number
  max_force: number
  ppm: number
}

export interface SessionState {
  active: boolean
  elapsed_sec: number
  left: HandState
  right: HandState
  combined: CombinedStats
  paused: boolean
}

export const defaultHandState: HandState = {
  connected: false,
  battery: 0,
  packet_loss: 0,
  punch_count: 0,
  punch_breakdown: {},
  max_force: 0,
  avg_force: 0,
  ppm: 0,
  recent_punches: [],
}

export const defaultSession: SessionState = {
  active: false,
  elapsed_sec: 0,
  left: { ...defaultHandState },
  right: { ...defaultHandState },
  combined: { total_punches: 0, avg_force: 0, max_force: 0, ppm: 0 },
  paused: false,
}
