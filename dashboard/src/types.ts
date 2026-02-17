// SessionState mirrors the Go struct broadcast over WebSocket.
export interface PunchEvent {
  force: number  // m/s²
  ts: number     // ESP32 millis
  count: number
}

export interface SessionState {
  active: boolean
  punch_count: number
  max_force: number
  avg_force: number
  ppm: number
  recent_punches: PunchEvent[]
  elapsed_sec: number
}

export const defaultSession: SessionState = {
  active: false,
  punch_count: 0,
  max_force: 0,
  avg_force: 0,
  ppm: 0,
  recent_punches: [],
  elapsed_sec: 0,
}
