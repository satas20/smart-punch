import React from 'react'
import { SessionState } from '../types'
import { GloveCard } from './GloveCard'

interface Props {
  state: SessionState
  connected: boolean
  roundDuration: number
  setRoundDuration: (duration: number) => void
  onStart: () => void
}

const ROUND_OPTIONS = [
  { label: '1:00', value: 60 },
  { label: '2:00', value: 120 },
  { label: '3:00', value: 180 },
  { label: '5:00', value: 300 },
]

export function PreTrainingView({ state, connected, roundDuration, setRoundDuration, onStart }: Props) {
  const anyGloveConnected = state.left.connected || state.right.connected
  const bothCalibrated = 
    (!state.left.connected || state.left.calibrated) && 
    (!state.right.connected || state.right.calibrated)
  const canStart = anyGloveConnected && bothCalibrated

  // Warning message
  let warningMessage = ''
  if (!connected) {
    warningMessage = 'Server disconnected. Waiting for connection...'
  } else if (!anyGloveConnected) {
    warningMessage = 'No gloves connected. Power on your FighterLink gloves.'
  } else if (!bothCalibrated) {
    warningMessage = 'Hold gloves still to calibrate before starting.'
  } else if (!state.left.connected || !state.right.connected) {
    warningMessage = `Only ${state.left.connected ? 'left' : 'right'} glove connected. You can start with one glove.`
  }

  return (
    <div style={styles.container}>
      {/* Header */}
      <div style={styles.header}>
        <h1 style={styles.title}>FIGHTERLINK</h1>
        <div style={styles.serverStatus}>
          <span style={{
            ...styles.serverDot,
            background: connected ? '#22c55e' : '#ef4444',
          }} />
          <span style={styles.serverLabel}>{connected ? 'CONNECTED' : 'DISCONNECTED'}</span>
        </div>
      </div>

      {/* Glove Cards */}
      <div style={styles.gloveContainer}>
        <GloveCard label="LEFT" hand={state.left} />
        <GloveCard label="RIGHT" hand={state.right} />
      </div>

      {/* Round Duration Selector */}
      <div style={styles.configSection}>
        <span style={styles.configLabel}>ROUND DURATION</span>
        <div style={styles.roundButtons}>
          {ROUND_OPTIONS.map(opt => (
            <button
              key={opt.value}
              style={{
                ...styles.roundBtn,
                ...(roundDuration === opt.value ? styles.roundBtnActive : {}),
              }}
              onClick={() => setRoundDuration(opt.value)}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {/* Warning Banner */}
      {warningMessage && (
        <div style={{
          ...styles.warning,
          borderColor: !anyGloveConnected || !connected ? '#ef4444' : '#f59e0b',
          color: !anyGloveConnected || !connected ? '#ef4444' : '#f59e0b',
        }}>
          {warningMessage}
        </div>
      )}

      {/* Start Button */}
      <button
        style={{
          ...styles.startBtn,
          ...(canStart ? {} : styles.startBtnDisabled),
        }}
        onClick={onStart}
        disabled={!canStart}
      >
        START SESSION
      </button>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    minHeight: '100vh',
    padding: '40px 20px',
    background: '#0a0a0a',
  },
  header: {
    textAlign: 'center',
    marginBottom: '40px',
  },
  title: {
    fontSize: '28px',
    fontWeight: 700,
    letterSpacing: '8px',
    color: '#f0f0f0',
    marginBottom: '12px',
  },
  serverStatus: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '8px',
  },
  serverDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
  },
  serverLabel: {
    fontSize: '11px',
    letterSpacing: '2px',
    color: '#555',
  },
  gloveContainer: {
    display: 'flex',
    gap: '24px',
    marginBottom: '32px',
    flexWrap: 'wrap',
    justifyContent: 'center',
  },
  configSection: {
    textAlign: 'center',
    marginBottom: '24px',
  },
  configLabel: {
    fontSize: '11px',
    letterSpacing: '2px',
    color: '#555',
    display: 'block',
    marginBottom: '12px',
  },
  roundButtons: {
    display: 'flex',
    gap: '8px',
  },
  roundBtn: {
    padding: '10px 20px',
    background: '#1e1e1e',
    border: '1px solid #333',
    borderRadius: '6px',
    color: '#888',
    fontSize: '14px',
    fontWeight: 600,
    cursor: 'pointer',
    transition: 'all 0.2s',
  },
  roundBtnActive: {
    background: '#ff4d4d',
    borderColor: '#ff4d4d',
    color: '#fff',
  },
  warning: {
    padding: '12px 24px',
    border: '1px solid',
    borderRadius: '8px',
    fontSize: '13px',
    marginBottom: '24px',
    textAlign: 'center',
    maxWidth: '500px',
  },
  startBtn: {
    padding: '16px 48px',
    background: '#ff4d4d',
    border: 'none',
    borderRadius: '8px',
    color: '#fff',
    fontSize: '16px',
    fontWeight: 700,
    letterSpacing: '3px',
    cursor: 'pointer',
    transition: 'all 0.2s',
  },
  startBtnDisabled: {
    background: '#2a2a2a',
    color: '#444',
    cursor: 'not-allowed',
  },
}
