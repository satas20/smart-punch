import { useBoxingSocket } from './hooks/useBoxingSocket'
import { PunchCounter } from './components/PunchCounter'
import { StatsBar } from './components/StatsBar'
import { ForceChart } from './components/ForceChart'

export default function App() {
  const { state, connected, startSession, resetSession } = useBoxingSocket()

  return (
    <div style={styles.root}>
      {/* ── Header ── */}
      <header style={styles.header}>
        <div style={styles.headerLeft}>
          <span style={styles.logo}>BOXING ANALYTICS</span>
          <span style={{
            ...styles.connDot,
            background: connected ? '#22c55e' : '#ef4444',
          }} />
          <span style={styles.connLabel}>
            {connected ? 'LIVE' : 'DISCONNECTED'}
          </span>
        </div>
        <div style={styles.headerRight}>
          <button
            style={{
              ...styles.btn,
              ...(state.active ? styles.btnDisabled : styles.btnStart),
            }}
            onClick={startSession}
            disabled={state.active}
          >
            START
          </button>
          <button
            style={{
              ...styles.btn,
              ...styles.btnReset,
            }}
            onClick={resetSession}
          >
            RESET
          </button>
        </div>
      </header>

      {/* ── Punch counter ── */}
      <PunchCounter count={state.punch_count} active={state.active} />

      {/* ── Stats + Chart ── */}
      <div style={styles.bottom}>
        <StatsBar
          avgForce={state.avg_force}
          maxForce={state.max_force}
          ppm={state.ppm}
          elapsedSec={state.elapsed_sec}
        />
        <ForceChart punches={state.recent_punches} />
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
  },
  connLabel: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
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
  bottom: {
    display: 'flex',
    flex: 1,
    minHeight: 0,
  },
}
