import React from 'react'
import { SessionState, HandState } from '../types'
import { RoundTimer } from './RoundTimer'
import { PunchBreakdown, TYPE_COLORS } from './PunchBreakdown'
import { ForceChart } from './ForceChart'

interface Props {
  state: SessionState
  roundDuration: number
  onPause: () => void
  onResume: () => void
  onStop: () => void
}

// Mini glove status badge for header
function GloveMini({ label, hand }: { label: string; hand: HandState }) {
  let color = '#333'
  if (hand.connected) {
    color = hand.calibrated ? '#22c55e' : '#f59e0b'
  }
  
  return (
    <div style={styles.gloveMini}>
      <span style={{ ...styles.gloveDot, background: color }} />
      <span style={styles.gloveLabel}>{label}</span>
      {hand.connected && (
        <span style={styles.glovePunches}>{hand.punch_count}</span>
      )}
    </div>
  )
}

export function LiveTrainingView({ state, roundDuration, onPause, onResume, onStop }: Props) {
  // Combine recent punches from both hands
  const allPunches = [...state.left.recent_punches, ...state.right.recent_punches]
    .sort((a, b) => a.count - b.count)
    .slice(-50)

  const lastPunchType = allPunches.length > 0
    ? allPunches[allPunches.length - 1].type
    : undefined

  return (
    <div style={styles.container}>
      {/* Header */}
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <span style={styles.logo}>FIGHTERLINK</span>
          <GloveMini label="L" hand={state.left} />
          <GloveMini label="R" hand={state.right} />
        </div>
        
        <div style={styles.headerRight}>
          {state.paused ? (
            <button style={{ ...styles.btn, ...styles.btnResume }} onClick={onResume}>
              RESUME
            </button>
          ) : (
            <button style={{ ...styles.btn, ...styles.btnPause }} onClick={onPause}>
              PAUSE
            </button>
          )}
          <button style={{ ...styles.btn, ...styles.btnStop }} onClick={onStop}>
            STOP
          </button>
        </div>
      </header>

      {/* Paused Banner */}
      {state.paused && (
        <div style={styles.pausedBanner}>
          SESSION PAUSED
        </div>
      )}

      {/* Main Content */}
      <div style={styles.main}>
        {/* Left Column: Timer + Punch Counter */}
        <div style={styles.leftCol}>
          <RoundTimer 
            elapsedSec={state.elapsed_sec} 
            roundDuration={roundDuration}
            paused={state.paused}
          />
          
          {/* Big Punch Counter */}
          <div style={styles.punchCounter}>
            <div style={styles.punchCount}>{state.combined.total_punches}</div>
            <div style={styles.punchLabel}>PUNCHES</div>
            {lastPunchType && (
              <div style={{
                ...styles.lastPunchType,
                color: TYPE_COLORS[lastPunchType] || '#888',
              }}>
                {lastPunchType.toUpperCase()}
              </div>
            )}
          </div>

          {/* Punch Rate */}
          <div style={styles.punchRate}>
            <span style={styles.rateValue}>{state.combined.pps.toFixed(1)}</span>
            <span style={styles.rateLabel}>punches/sec</span>
          </div>
        </div>

        {/* Right Column: Stats + Chart */}
        <div style={styles.rightCol}>
          {/* Stats Row */}
          <div style={styles.statsRow}>
            <StatCard label="AVG FORCE" value={state.combined.avg_force.toFixed(1)} unit="m/s²" />
            <StatCard label="MAX FORCE" value={state.combined.max_force.toFixed(1)} unit="m/s²" />
            <StatCard label="LEFT" value={state.left.punch_count.toString()} />
            <StatCard label="RIGHT" value={state.right.punch_count.toString()} />
            <StatCard 
              label="INTENSITY" 
              value={state.combined.intensity_score.toLocaleString()} 
              highlight
            />
          </div>

          {/* Punch Type Breakdown */}
          <PunchBreakdown 
            left={state.left.punch_breakdown} 
            right={state.right.punch_breakdown} 
          />

          {/* Force Chart */}
          <div style={styles.chartContainer}>
            <ForceChart punches={allPunches} />
          </div>
        </div>
      </div>
    </div>
  )
}

function StatCard({ label, value, unit, highlight }: { 
  label: string
  value: string
  unit?: string
  highlight?: boolean
}) {
  return (
    <div style={{
      ...styles.statCard,
      ...(highlight ? styles.statCardHighlight : {}),
    }}>
      <div style={styles.statLabel}>{label}</div>
      <div style={{
        ...styles.statValue,
        ...(highlight ? styles.statValueHighlight : {}),
      }}>
        {value}
        {unit && <span style={styles.statUnit}> {unit}</span>}
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
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
    gap: '16px',
  },
  logo: {
    fontSize: '13px',
    fontWeight: 700,
    letterSpacing: '3px',
    color: '#f0f0f0',
  },
  gloveMini: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    padding: '4px 10px',
    background: '#111',
    border: '1px solid #1e1e1e',
    borderRadius: '4px',
  },
  gloveDot: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
  },
  gloveLabel: {
    fontSize: '10px',
    fontWeight: 700,
    color: '#888',
  },
  glovePunches: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
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
  btnPause: {
    background: '#f59e0b',
    color: '#000',
  },
  btnResume: {
    background: '#22c55e',
    color: '#000',
  },
  btnStop: {
    background: '#ef4444',
    color: '#fff',
  },
  pausedBanner: {
    background: '#f59e0b22',
    borderBottom: '1px solid #f59e0b44',
    color: '#f59e0b',
    fontSize: '12px',
    fontWeight: 600,
    letterSpacing: '3px',
    textAlign: 'center',
    padding: '8px',
  },
  main: {
    display: 'flex',
    flex: 1,
    minHeight: 0,
  },
  leftCol: {
    width: '320px',
    borderRight: '1px solid #1e1e1e',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: '24px',
    flexShrink: 0,
  },
  punchCounter: {
    textAlign: 'center',
    marginTop: '32px',
  },
  punchCount: {
    fontSize: '96px',
    fontWeight: 700,
    color: '#f0f0f0',
    lineHeight: 1,
    fontVariantNumeric: 'tabular-nums',
  },
  punchLabel: {
    fontSize: '14px',
    fontWeight: 600,
    letterSpacing: '4px',
    color: '#555',
    marginTop: '8px',
  },
  lastPunchType: {
    fontSize: '18px',
    fontWeight: 700,
    letterSpacing: '3px',
    marginTop: '16px',
  },
  punchRate: {
    marginTop: 'auto',
    textAlign: 'center',
    padding: '16px 24px',
    background: '#111',
    borderRadius: '8px',
  },
  rateValue: {
    fontSize: '32px',
    fontWeight: 700,
    color: '#ff4d4d',
    fontVariantNumeric: 'tabular-nums',
  },
  rateLabel: {
    fontSize: '11px',
    letterSpacing: '2px',
    color: '#555',
    display: 'block',
    marginTop: '4px',
  },
  rightCol: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    minWidth: 0,
  },
  statsRow: {
    display: 'flex',
    borderBottom: '1px solid #1e1e1e',
  },
  statCard: {
    flex: 1,
    padding: '16px',
    borderRight: '1px solid #1e1e1e',
  },
  statCardHighlight: {
    background: '#1a0a0a',
  },
  statLabel: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
    marginBottom: '6px',
  },
  statValue: {
    fontSize: '20px',
    fontWeight: 600,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
  },
  statValueHighlight: {
    color: '#ff4d4d',
  },
  statUnit: {
    fontSize: '11px',
    color: '#555',
    fontWeight: 400,
  },
  chartContainer: {
    flex: 1,
    minHeight: 0,
  },
}
