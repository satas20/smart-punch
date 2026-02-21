import React from 'react'
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip } from 'recharts'
import { TYPE_COLORS } from './PunchBreakdown'

interface Props {
  breakdown: Record<string, number>
}

const PUNCH_TYPES = ['straight', 'hook', 'uppercut', 'unknown']

export function PunchPieChart({ breakdown }: Props) {
  const data = PUNCH_TYPES
    .filter(type => (breakdown[type] ?? 0) > 0)
    .map(type => ({
      name: type.charAt(0).toUpperCase() + type.slice(1),
      value: breakdown[type] ?? 0,
      color: TYPE_COLORS[type] || '#444',
    }))

  const total = data.reduce((sum, d) => sum + d.value, 0)

  if (total === 0) {
    return (
      <div style={styles.empty}>
        No punches recorded
      </div>
    )
  }

  return (
    <div style={styles.container}>
      <h3 style={styles.title}>PUNCH BREAKDOWN</h3>
      <div style={styles.chartWrapper}>
        <ResponsiveContainer width="100%" height={250}>
          <PieChart>
            <Pie
              data={data}
              cx="50%"
              cy="50%"
              innerRadius={60}
              outerRadius={100}
              paddingAngle={2}
              dataKey="value"
              label={({ name, percent }) => `${name} ${(percent * 100).toFixed(0)}%`}
              labelLine={{ stroke: '#444', strokeWidth: 1 }}
            >
              {data.map((entry, index) => (
                <Cell key={`cell-${index}`} fill={entry.color} />
              ))}
            </Pie>
            <Tooltip
              formatter={(value: number) => [`${value} punches`, '']}
              contentStyle={{
                background: '#111',
                border: '1px solid #222',
                borderRadius: '4px',
              }}
            />
          </PieChart>
        </ResponsiveContainer>
      </div>
      
      {/* Legend below chart */}
      <div style={styles.legend}>
        {data.map((item) => (
          <div key={item.name} style={styles.legendItem}>
            <span style={{ ...styles.legendDot, background: item.color }} />
            <span style={styles.legendName}>{item.name}</span>
            <span style={styles.legendValue}>{item.value}</span>
            <span style={styles.legendPercent}>
              ({((item.value / total) * 100).toFixed(0)}%)
            </span>
          </div>
        ))}
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
    marginBottom: '16px',
    textAlign: 'center',
  },
  chartWrapper: {
    width: '100%',
    height: '250px',
  },
  legend: {
    display: 'flex',
    flexDirection: 'column',
    gap: '8px',
    marginTop: '16px',
  },
  legendItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  },
  legendDot: {
    width: '12px',
    height: '12px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  legendName: {
    fontSize: '13px',
    color: '#888',
    flex: 1,
  },
  legendValue: {
    fontSize: '14px',
    fontWeight: 600,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
  },
  legendPercent: {
    fontSize: '12px',
    color: '#555',
    width: '40px',
    textAlign: 'right',
  },
  empty: {
    padding: '40px',
    textAlign: 'center',
    color: '#444',
    fontSize: '14px',
  },
}
