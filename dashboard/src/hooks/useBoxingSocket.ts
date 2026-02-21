import { useEffect, useRef, useState, useCallback } from 'react'
import { SessionState, defaultSession, AppPhase } from '../types'

const WS_URL = '/ws'            // proxied by Vite in dev, direct in prod
const RECONNECT_DELAY_MS = 2000 // retry after 2s on disconnect

interface UseBoxingSocket {
  // Server state
  state: SessionState
  connected: boolean
  
  // Local UI state
  phase: AppPhase
  roundDuration: number
  setRoundDuration: (duration: number) => void
  
  // Snapshot of final state when session ends (for post-training view)
  finalState: SessionState | null
  
  // Actions
  startSession: () => void
  pauseSession: () => void
  resumeSession: () => void
  stopSession: () => void
  resetSession: () => void
}

export function useBoxingSocket(): UseBoxingSocket {
  const [state, setState] = useState<SessionState>(defaultSession)
  const [connected, setConnected] = useState(false)
  const [phase, setPhase] = useState<AppPhase>('pre')
  const [roundDuration, setRoundDuration] = useState(180) // 3 minutes default
  const [finalState, setFinalState] = useState<SessionState | null>(null)
  
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const prevActiveRef = useRef(false)

  const connect = useCallback(() => {
    // Build absolute WS URL for production, relative in dev (Vite proxy)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    const url = `${protocol}//${host}${WS_URL}`

    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      console.log('[WS] Connected to server')
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
        reconnectTimer.current = null
      }
    }

    ws.onmessage = (evt) => {
      try {
        const data = JSON.parse(evt.data) as SessionState
        setState(data)
      } catch (e) {
        console.warn('[WS] Failed to parse message:', evt.data)
      }
    }

    ws.onerror = (err) => {
      console.error('[WS] Error:', err)
    }

    ws.onclose = () => {
      setConnected(false)
      console.warn('[WS] Disconnected. Reconnecting in', RECONNECT_DELAY_MS, 'ms...')
      reconnectTimer.current = setTimeout(connect, RECONNECT_DELAY_MS)
    }
  }, [])

  // Track phase changes based on server state
  useEffect(() => {
    const wasActive = prevActiveRef.current
    const isActive = state.active
    
    if (!wasActive && isActive) {
      // Session just started
      setPhase('live')
      setFinalState(null)
    }
    
    prevActiveRef.current = isActive
  }, [state.active])

  useEffect(() => {
    connect()
    return () => {
      wsRef.current?.close()
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
    }
  }, [connect])

  const startSession = useCallback(async () => {
    try {
      await fetch('/api/session/start', { method: 'POST' })
      setPhase('live')
      setFinalState(null)
    } catch (e) {
      console.error('[API] session/start failed:', e)
    }
  }, [])

  const pauseSession = useCallback(async () => {
    try {
      await fetch('/api/session/pause', { method: 'POST' })
    } catch (e) {
      console.error('[API] session/pause failed:', e)
    }
  }, [])

  const resumeSession = useCallback(async () => {
    try {
      await fetch('/api/session/resume', { method: 'POST' })
    } catch (e) {
      console.error('[API] session/resume failed:', e)
    }
  }, [])

  const stopSession = useCallback(async () => {
    // Save current state as final state before stopping
    setFinalState({ ...state })
    setPhase('post')
    
    try {
      await fetch('/api/session/stop', { method: 'POST' })
    } catch (e) {
      console.error('[API] session/stop failed:', e)
    }
  }, [state])

  const resetSession = useCallback(async () => {
    setPhase('pre')
    setFinalState(null)
    
    try {
      await fetch('/api/session/reset', { method: 'POST' })
    } catch (e) {
      console.error('[API] session/reset failed:', e)
    }
  }, [])

  return {
    state,
    connected,
    phase,
    roundDuration,
    setRoundDuration,
    finalState,
    startSession,
    pauseSession,
    resumeSession,
    stopSession,
    resetSession,
  }
}
