import React from 'react'
import { useBoxingSocket } from './hooks/useBoxingSocket'
import { PunchCounter } from './components/PunchCounter'
import { StatsBar } from './components/StatsBar'
import { ForceChart } from './components/ForceChart'
import { PunchBreakdown } from './components/PunchBreakdown'
import type { HandState } from './types'

// ─── Glove badge ─────────────────────────────────────────

function GloveBadge({ label, hand }: { label: string; hand: HandState }) {
  return (
    <div style={styles.gloveBadge}>
      <span style={{
        ...styles.gloveDot,
        background: hand.connected ? '#22c55e' : '#333',
      }} />
      <span style={styles.gloveLabel}>{label}</span>
      {hand.connected && (
        <span style={styles.gloveSub}>
          {hand.punch_count}  {hand.battery}%
        </span>
      )}
    </div>
  )
}

// ─── App ─────────────────────────────────────────────────

export default function App() {
  const { state, connected, startSession, resetSession } = useBoxingSocket()

  // Combine recent punches from both hands, sorted by count
  const allPunches = [...state.left.recent_punches, ...state.right.recent_punches]
    .sort((a, b) => a.count - b.count)
    .slice(-50)

  const lastPunchType = allPunches.length > 0
    ? allPunches[allPunches.length - 1].type
    : undefined

  const anyGloveConnected = state.left.connected || state.right.connected

  return (
    <div style={styles.root}>

      {/* ── Header ── */}
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <span style={styles.logo}>BOXING ANALYTICS</span>

          {/* WS connection */}
          <span style={{ ...styles.connDot, background: connected ? '#22c55e' : '#ef4444' }} />
          <span style={styles.connLabel}>{connected ? 'LIVE' : 'DISCONNECTED'}</span>

          {/* Glove badges */}
          <div style={styles.gloveBadges}>
            <GloveBadge label="L" hand={state.left} />
            <GloveBadge label="R" hand={state.right} />
          </div>
        </div>

        <div style={styles.headerRight}>
          <button
            style={{ ...styles.btn, ...(state.active ? styles.btnDisabled : styles.btnStart) }}
            onClick={startSession}
            disabled={state.active || !anyGloveConnected}
          >
            START
          </button>
          <button style={{ ...styles.btn, ...styles.btnReset }} onClick={resetSession}>
            RESET
          </button>
        </div>
      </header>

      {/* ── Paused banner ── */}
      {state.paused && (
        <div style={styles.pausedBanner}>
          PAUSED — glove disconnected mid-session
        </div>
      )}

      {/* ── Punch counter + last type ── */}
      <PunchCounter
        count={state.combined.total_punches}
        active={state.active}
        lastType={lastPunchType}
      />

      {/* ── Punch type breakdown ── */}
      <PunchBreakdown
        left={state.left.punch_breakdown}
        right={state.right.punch_breakdown}
      />

      {/* ── Stats + Chart ── */}
      <div style={styles.bottom}>
        <StatsBar
          avgForce={state.combined.avg_force}
          maxForce={state.combined.max_force}
          ppm={state.combined.ppm}
          elapsedSec={state.elapsed_sec}
        />
        <ForceChart punches={allPunches} />
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  root: {
    display: 'flex',
    flexDirection: 'column',
    height: '100vh',
    background: '#0a0a0a',
    overflow: 'hidden',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '0 24px',
    height: '56px',
    borderBottom: '1px solid #1e1e1e',
    flexShrink: 0,
  },
  headerLeft: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  },
  logo: {
    fontSize: '13px',
    fontWeight: 700,
    letterSpacing: '3px',
    color: '#f0f0f0',
  },
  connDot: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    display: 'inline-block',
    flexShrink: 0,
  },
  connLabel: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
  },
  gloveBadges: {
    display: 'flex',
    gap: '8px',
    marginLeft: '8px',
  },
  gloveBadge: {
    display: 'flex',
    alignItems: 'center',
    gap: '5px',
    background: '#111',
    border: '1px solid #1e1e1e',
    borderRadius: '4px',
    padding: '3px 8px',
  },
  gloveDot: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  gloveLabel: {
    fontSize: '10px',
    fontWeight: 700,
    letterSpacing: '1px',
    color: '#aaa',
  },
  gloveSub: {
    fontSize: '10px',
    color: '#555',
    letterSpacing: '1px',
  },
  headerRight: {
    display: 'flex',
    gap: '8px',
  },
  btn: {
    padding: '8px 20px',
    border: 'none',
    borderRadius: '4px',
    fontSize: '12px',
    fontWeight: 700,
    letterSpacing: '2px',
    cursor: 'pointer',
    transition: 'opacity 0.2s',
  },
  btnStart: {
    background: '#ff4d4d',
    color: '#fff',
  },
  btnDisabled: {
    background: '#2a2a2a',
    color: '#444',
    cursor: 'not-allowed',
  },
  btnReset: {
    background: '#1e1e1e',
    color: '#aaa',
  },
  pausedBanner: {
    background: '#1a0000',
    borderBottom: '1px solid #3a0000',
    color: '#ff4d4d',
    fontSize: '11px',
    letterSpacing: '2px',
    textAlign: 'center',
    padding: '6px',
    flexShrink: 0,
  },
  bottom: {
    display: 'flex',
    flex: 1,
    minHeight: 0,
  },
}
