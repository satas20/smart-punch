import React from 'react'
import { SessionState, formatTime } from '../types'
import { PunchPieChart } from './PunchPieChart'
import { HandComparison } from './HandComparison'
import { ForceChart } from './ForceChart'

interface Props {
  state: SessionState
  onNewSession: () => void
}

export function PostTrainingView({ state, onNewSession }: Props) {
  // Combine punch breakdowns from both hands
  const combinedBreakdown: Record<string, number> = {}
  for (const [type, count] of Object.entries(state.left.punch_breakdown)) {
    combinedBreakdown[type] = (combinedBreakdown[type] ?? 0) + count
  }
  for (const [type, count] of Object.entries(state.right.punch_breakdown)) {
    combinedBreakdown[type] = (combinedBreakdown[type] ?? 0) + count
  }

  // Combine recent punches for the force chart
  const allPunches = [...state.left.recent_punches, ...state.right.recent_punches]
    .sort((a, b) => a.count - b.count)

  return (
    <div style={styles.container}>
      {/* Header */}
      <header style={styles.header}>
        <div>
          <h1 style={styles.title}>SESSION COMPLETE</h1>
          <p style={styles.subtitle}>Great workout! Here's your summary.</p>
        </div>
        <button style={styles.newSessionBtn} onClick={onNewSession}>
          NEW SESSION
        </button>
      </header>

      {/* Summary Cards */}
      <div style={styles.summaryRow}>
        <SummaryCard 
          label="TOTAL PUNCHES" 
          value={state.combined.total_punches.toString()} 
          icon="👊"
        />
        <SummaryCard 
          label="DURATION" 
          value={formatTime(state.elapsed_sec)} 
          icon="⏱️"
        />
        <SummaryCard 
          label="AVG FORCE" 
          value={`${state.combined.avg_force.toFixed(1)} m/s²`} 
          icon="💪"
        />
        <SummaryCard 
          label="MAX FORCE" 
          value={`${state.combined.max_force.toFixed(1)} m/s²`} 
          icon="🔥"
        />
        <SummaryCard 
          label="INTENSITY SCORE" 
          value={state.combined.intensity_score.toLocaleString()} 
          icon="⚡"
          highlight
        />
      </div>

      {/* Main Content */}
      <div style={styles.content}>
        {/* Left Column: Pie Chart */}
        <div style={styles.leftCol}>
          <PunchPieChart breakdown={combinedBreakdown} />
        </div>

        {/* Right Column: Hand Comparison + Force Chart */}
        <div style={styles.rightCol}>
          <HandComparison left={state.left} right={state.right} />
          
          <div style={styles.chartSection}>
            <h3 style={styles.sectionTitle}>FORCE OVER TIME</h3>
            <div style={styles.chartWrapper}>
              <ForceChart punches={allPunches} />
            </div>
          </div>
        </div>
      </div>

      {/* Footer Stats */}
      <div style={styles.footer}>
        <FooterStat label="Punches/Min" value={state.combined.ppm.toFixed(1)} />
        <FooterStat label="Left Hand" value={`${state.left.punch_count} punches`} />
        <FooterStat label="Right Hand" value={`${state.right.punch_count} punches`} />
        <FooterStat label="Session Time" value={formatTime(state.elapsed_sec)} />
      </div>
    </div>
  )
}

function SummaryCard({ label, value, icon, highlight }: {
  label: string
  value: string
  icon: string
  highlight?: boolean
}) {
  return (
    <div style={{
      ...styles.summaryCard,
      ...(highlight ? styles.summaryCardHighlight : {}),
    }}>
      <span style={styles.summaryIcon}>{icon}</span>
      <span style={styles.summaryLabel}>{label}</span>
      <span style={{
        ...styles.summaryValue,
        ...(highlight ? styles.summaryValueHighlight : {}),
      }}>{value}</span>
    </div>
  )
}

function FooterStat({ label, value }: { label: string; value: string }) {
  return (
    <div style={styles.footerStat}>
      <span style={styles.footerLabel}>{label}</span>
      <span style={styles.footerValue}>{value}</span>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    minHeight: '100vh',
    background: '#0a0a0a',
    padding: '24px',
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '32px',
  },
  title: {
    fontSize: '24px',
    fontWeight: 700,
    letterSpacing: '4px',
    color: '#f0f0f0',
    marginBottom: '8px',
  },
  subtitle: {
    fontSize: '14px',
    color: '#555',
  },
  newSessionBtn: {
    padding: '12px 32px',
    background: '#ff4d4d',
    border: 'none',
    borderRadius: '8px',
    color: '#fff',
    fontSize: '14px',
    fontWeight: 700,
    letterSpacing: '2px',
    cursor: 'pointer',
  },
  summaryRow: {
    display: 'flex',
    gap: '16px',
    marginBottom: '32px',
    flexWrap: 'wrap',
  },
  summaryCard: {
    flex: 1,
    minWidth: '150px',
    padding: '20px',
    background: '#111',
    border: '1px solid #1e1e1e',
    borderRadius: '12px',
    textAlign: 'center',
  },
  summaryCardHighlight: {
    background: '#1a0a0a',
    borderColor: '#ff4d4d33',
  },
  summaryIcon: {
    fontSize: '24px',
    display: 'block',
    marginBottom: '8px',
  },
  summaryLabel: {
    fontSize: '10px',
    fontWeight: 600,
    letterSpacing: '2px',
    color: '#555',
    display: 'block',
    marginBottom: '8px',
  },
  summaryValue: {
    fontSize: '24px',
    fontWeight: 700,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
  },
  summaryValueHighlight: {
    color: '#ff4d4d',
  },
  content: {
    display: 'flex',
    gap: '24px',
    marginBottom: '32px',
  },
  leftCol: {
    width: '350px',
    flexShrink: 0,
  },
  rightCol: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    gap: '24px',
  },
  chartSection: {
    background: '#111',
    borderRadius: '12px',
    border: '1px solid #1e1e1e',
    padding: '20px',
    flex: 1,
  },
  sectionTitle: {
    fontSize: '12px',
    fontWeight: 600,
    letterSpacing: '2px',
    color: '#555',
    marginBottom: '16px',
  },
  chartWrapper: {
    height: '200px',
  },
  footer: {
    display: 'flex',
    gap: '32px',
    padding: '20px',
    background: '#111',
    borderRadius: '12px',
    border: '1px solid #1e1e1e',
    justifyContent: 'center',
  },
  footerStat: {
    textAlign: 'center',
  },
  footerLabel: {
    fontSize: '10px',
    fontWeight: 600,
    letterSpacing: '1px',
    color: '#555',
    display: 'block',
    marginBottom: '4px',
  },
  footerValue: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#888',
  },
}
