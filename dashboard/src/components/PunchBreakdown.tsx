import React from 'react'

export const TYPE_COLORS: Record<string, string> = {
  straight: '#3b82f6',
  hook:     '#f59e0b',
  uppercut: '#a855f7',
  unknown:  '#444',
}

const PUNCH_TYPES = ['straight', 'hook', 'uppercut']

interface Props {
  left:  Record<string, number>
  right: Record<string, number>
}

function mergeCounts(
  a: Record<string, number>,
  b: Record<string, number>,
): Record<string, number> {
  const result: Record<string, number> = {}
  for (const t of [...PUNCH_TYPES, 'unknown']) {
    result[t] = (a[t] ?? 0) + (b[t] ?? 0)
  }
  return result
}

export function PunchBreakdown({ left, right }: Props) {
  const combined = mergeCounts(left, right)
  const total = PUNCH_TYPES.reduce((s, t) => s + (combined[t] ?? 0), 0)

  return (
    <div style={styles.wrapper}>
      {PUNCH_TYPES.map(type => {
        const count = combined[type] ?? 0
        const pct   = total > 0 ? (count / total) * 100 : 0
        const color = TYPE_COLORS[type]
        return (
          <div key={type} style={styles.item}>
            <div style={styles.labelRow}>
              <span style={{ ...styles.dot, background: color }} />
              <span style={styles.typeName}>{type.toUpperCase()}</span>
              <span style={{ ...styles.count, color }}>{count}</span>
            </div>
            <div style={styles.barBg}>
              <div style={{ ...styles.barFill, width: `${pct}%`, background: color }} />
            </div>
          </div>
        )
      })}
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  wrapper: {
    display: 'flex',
    gap: '1px',
    borderBottom: '1px solid #1e1e1e',
    background: '#0d0d0d',
    flexShrink: 0,
  },
  item: {
    flex: 1,
    padding: '10px 16px',
    borderRight: '1px solid #1e1e1e',
  },
  labelRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '6px',
    marginBottom: '6px',
  },
  dot: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  typeName: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
    flex: 1,
  },
  count: {
    fontSize: '18px',
    fontWeight: 700,
    fontVariantNumeric: 'tabular-nums',
    lineHeight: 1,
  },
  barBg: {
    height: '3px',
    background: '#1e1e1e',
    borderRadius: '2px',
    overflow: 'hidden',
  },
  barFill: {
    height: '100%',
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
}
