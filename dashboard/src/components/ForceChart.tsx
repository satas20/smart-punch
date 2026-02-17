import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ReferenceLine,
  ResponsiveContainer,
} from 'recharts'
import { PunchEvent } from '../types'

interface Props {
  punches: PunchEvent[]
  threshold?: number
}

const THRESHOLD_DEFAULT = 35 // m/s²

// Custom tooltip to keep things clean
function CustomTooltip({ active, payload }: { active?: boolean; payload?: { value: number }[] }) {
  if (!active || !payload?.length) return null
  return (
    <div style={styles.tooltip}>
      <span style={{ color: '#ff4d4d' }}>{payload[0].value.toFixed(1)}</span>
      <span style={{ color: '#555', marginLeft: 4 }}>m/s²</span>
    </div>
  )
}

export function ForceChart({ punches, threshold = THRESHOLD_DEFAULT }: Props) {
  const data = punches.map((p) => ({
    count: p.count,
    force: Math.round(p.force * 10) / 10,
  }))

  const isEmpty = data.length === 0

  return (
    <div style={styles.wrapper}>
      <p style={styles.title}>FORCE PER PUNCH  <span style={styles.subtitle}>(last {punches.length} / 50)</span></p>

      {isEmpty ? (
        <div style={styles.empty}>No punches yet this session</div>
      ) : (
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={data} margin={{ top: 10, right: 24, left: 0, bottom: 10 }}>
            <CartesianGrid stroke="#1a1a1a" strokeDasharray="4 4" />
            <XAxis
              dataKey="count"
              tick={{ fill: '#444', fontSize: 11 }}
              label={{ value: 'Punch #', position: 'insideBottom', offset: -4, fill: '#444', fontSize: 11 }}
            />
            <YAxis
              tick={{ fill: '#444', fontSize: 11 }}
              domain={['auto', 'auto']}
              unit=" m/s²"
              width={64}
            />
            <Tooltip content={<CustomTooltip />} />
            {/* Threshold reference line */}
            <ReferenceLine
              y={threshold}
              stroke="#ff4d4d"
              strokeDasharray="6 3"
              label={{ value: `Threshold ${threshold}`, fill: '#ff4d4d', fontSize: 10, position: 'right' }}
            />
            <Line
              type="monotone"
              dataKey="force"
              stroke="#ff4d4d"
              strokeWidth={2}
              dot={{ r: 3, fill: '#ff4d4d', strokeWidth: 0 }}
              activeDot={{ r: 5, fill: '#fff' }}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      )}
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  wrapper: {
    flex: 1,
    padding: '20px 16px 12px',
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
  title: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
    textTransform: 'uppercase',
    marginBottom: '12px',
  },
  subtitle: {
    fontSize: '10px',
    color: '#333',
    letterSpacing: 0,
    textTransform: 'none',
  },
  empty: {
    flex: 1,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#2a2a2a',
    fontSize: '14px',
  },
  tooltip: {
    background: '#111',
    border: '1px solid #222',
    borderRadius: '4px',
    padding: '6px 10px',
    fontSize: '13px',
  },
}
