import React from 'react'
import { HandState } from '../types'
import { TYPE_COLORS } from './PunchBreakdown'

interface Props {
  left: HandState
  right: HandState
}

const PUNCH_TYPES = ['straight', 'hook', 'uppercut']

export function HandComparison({ left, right }: Props) {
  const totalPunches = left.punch_count + right.punch_count
  const leftPercent = totalPunches > 0 ? (left.punch_count / totalPunches) * 100 : 50
  const rightPercent = totalPunches > 0 ? (right.punch_count / totalPunches) * 100 : 50

  return (
    <div style={styles.container}>
      <h3 style={styles.title}>LEFT vs RIGHT COMPARISON</h3>
      
      {/* Visual Bar Comparison */}
      <div style={styles.barComparison}>
        <div style={styles.barLabels}>
          <span style={styles.barLabel}>LEFT ({left.punch_count})</span>
          <span style={styles.barLabel}>RIGHT ({right.punch_count})</span>
        </div>
        <div style={styles.comparisonBar}>
          <div style={{
            ...styles.leftBar,
            width: `${leftPercent}%`,
          }}>
            {leftPercent >= 15 && `${leftPercent.toFixed(0)}%`}
          </div>
          <div style={{
            ...styles.rightBar,
            width: `${rightPercent}%`,
          }}>
            {rightPercent >= 15 && `${rightPercent.toFixed(0)}%`}
          </div>
        </div>
      </div>

      {/* Stats Comparison */}
      <div style={styles.statsGrid}>
        <HandStats label="LEFT HAND" hand={left} color="#3b82f6" />
        <HandStats label="RIGHT HAND" hand={right} color="#ef4444" />
      </div>
    </div>
  )
}

function HandStats({ label, hand, color }: { label: string; hand: HandState; color: string }) {
  const total = Object.values(hand.punch_breakdown).reduce((a, b) => a + b, 0)
  
  return (
    <div style={styles.handCard}>
      <div style={{ ...styles.handHeader, borderColor: color }}>
        <span style={styles.handLabel}>{label}</span>
        <span style={{ ...styles.handCount, color }}>{hand.punch_count}</span>
      </div>
      
      <div style={styles.statRow}>
        <span style={styles.statLabel}>Avg Force</span>
        <span style={styles.statValue}>{hand.avg_force.toFixed(1)} m/s²</span>
      </div>
      
      <div style={styles.statRow}>
        <span style={styles.statLabel}>Max Force</span>
        <span style={styles.statValue}>{hand.max_force.toFixed(1)} m/s²</span>
      </div>
      
      <div style={styles.statRow}>
        <span style={styles.statLabel}>Punch/Min</span>
        <span style={styles.statValue}>{hand.ppm.toFixed(1)}</span>
      </div>

      {/* Punch Type Breakdown */}
      <div style={styles.breakdownSection}>
        {PUNCH_TYPES.map(type => {
          const count = hand.punch_breakdown[type] ?? 0
          const pct = total > 0 ? (count / total) * 100 : 0
          return (
            <div key={type} style={styles.breakdownRow}>
              <span style={{ ...styles.typeIndicator, background: TYPE_COLORS[type] }} />
              <span style={styles.typeName}>{type}</span>
              <span style={styles.typePercent}>{pct.toFixed(0)}%</span>
            </div>
          )
        })}
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    padding: '20px',
    background: '#111',
    borderRadius: '12px',
    border: '1px solid #1e1e1e',
  },
  title: {
    fontSize: '12px',
    fontWeight: 600,
    letterSpacing: '2px',
    color: '#555',
    marginBottom: '20px',
    textAlign: 'center',
  },
  barComparison: {
    marginBottom: '24px',
  },
  barLabels: {
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: '8px',
  },
  barLabel: {
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '1px',
    color: '#888',
  },
  comparisonBar: {
    display: 'flex',
    height: '32px',
    borderRadius: '4px',
    overflow: 'hidden',
    background: '#1e1e1e',
  },
  leftBar: {
    background: '#3b82f6',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '12px',
    fontWeight: 600,
    color: '#fff',
    transition: 'width 0.5s ease',
  },
  rightBar: {
    background: '#ef4444',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: '12px',
    fontWeight: 600,
    color: '#fff',
    transition: 'width 0.5s ease',
  },
  statsGrid: {
    display: 'grid',
    gridTemplateColumns: '1fr 1fr',
    gap: '16px',
  },
  handCard: {
    padding: '16px',
    background: '#0a0a0a',
    borderRadius: '8px',
  },
  handHeader: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingBottom: '12px',
    marginBottom: '12px',
    borderBottom: '2px solid',
  },
  handLabel: {
    fontSize: '11px',
    fontWeight: 600,
    letterSpacing: '1px',
    color: '#888',
  },
  handCount: {
    fontSize: '24px',
    fontWeight: 700,
    fontVariantNumeric: 'tabular-nums',
  },
  statRow: {
    display: 'flex',
    justifyContent: 'space-between',
    marginBottom: '8px',
  },
  statLabel: {
    fontSize: '11px',
    color: '#555',
  },
  statValue: {
    fontSize: '12px',
    fontWeight: 600,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
  },
  breakdownSection: {
    marginTop: '12px',
    paddingTop: '12px',
    borderTop: '1px solid #1e1e1e',
  },
  breakdownRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    marginBottom: '6px',
  },
  typeIndicator: {
    width: '8px',
    height: '8px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  typeName: {
    fontSize: '11px',
    color: '#666',
    flex: 1,
    textTransform: 'capitalize',
  },
  typePercent: {
    fontSize: '11px',
    fontWeight: 600,
    color: '#888',
  },
}
