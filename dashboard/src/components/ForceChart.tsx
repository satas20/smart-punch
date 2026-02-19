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
import { TYPE_COLORS } from './PunchBreakdown'

interface Props {
  punches: PunchEvent[]
  threshold?: number
}

const THRESHOLD_DEFAULT = 35 // m/s²

// Tooltip coloured by punch type
function CustomTooltip({ active, payload }: { active?: boolean; payload?: { value: number; payload?: { type?: string; hand?: string } }[] }) {
  if (!active || !payload?.length) return null
  const p = payload[0].payload
  const color = TYPE_COLORS[p?.type ?? ''] ?? '#ff4d4d'
  return (
    <div style={styles.tooltip}>
      <span style={{ color }}>{payload[0].value.toFixed(1)}</span>
      <span style={{ color: '#555', marginLeft: 4 }}>m/s²</span>
      {p?.type && (
        <span style={{ color, marginLeft: 8, fontSize: 10, letterSpacing: 1 }}>
          {p.type.toUpperCase()}
        </span>
      )}
      {p?.hand && (
        <span style={{ color: '#444', marginLeft: 6, fontSize: 10 }}>
          {p.hand[0].toUpperCase()}
        </span>
      )}
    </div>
  )
}

// Custom dot: coloured by punch type
function TypeDot(props: {
  cx?: number
  cy?: number
  payload?: { type?: string }
}) {
  const { cx, cy, payload } = props
  if (cx === undefined || cy === undefined) return null
  const color = TYPE_COLORS[payload?.type ?? ''] ?? '#ff4d4d'
  return <circle cx={cx} cy={cy} r={4} fill={color} />
}

// Custom active dot: white ring
function ActiveDot(props: { cx?: number; cy?: number; payload?: { type?: string } }) {
  const { cx, cy, payload } = props
  if (cx === undefined || cy === undefined) return null
  const color = TYPE_COLORS[payload?.type ?? ''] ?? '#ff4d4d'
  return <circle cx={cx} cy={cy} r={6} fill="#fff" stroke={color} strokeWidth={2} />
}

export function ForceChart({ punches, threshold = THRESHOLD_DEFAULT }: Props) {
  const data = punches.map((p) => ({
    count: p.count,
    force: Math.round(p.force * 10) / 10,
    type:  p.type,
    hand:  p.hand,
  }))

  const isEmpty = data.length === 0

  return (
    <div style={styles.wrapper}>
      <div style={styles.headerRow}>
        <p style={styles.title}>
          FORCE PER PUNCH
          <span style={styles.subtitle}> (last {punches.length} / 50)</span>
        </p>
        {/* Legend */}
        <div style={styles.legend}>
          {(['straight', 'hook', 'uppercut'] as const).map(t => (
            <span key={t} style={styles.legendItem}>
              <span style={{ ...styles.legendDot, background: TYPE_COLORS[t] }} />
              {t}
            </span>
          ))}
        </div>
      </div>

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
            <ReferenceLine
              y={threshold}
              stroke="#ff4d4d"
              strokeDasharray="6 3"
              label={{ value: `${threshold} threshold`, fill: '#ff4d4d', fontSize: 10, position: 'right' }}
            />
            <Line
              type="monotone"
              dataKey="force"
              stroke="#333"
              strokeWidth={1}
              dot={<TypeDot />}
              activeDot={<ActiveDot />}
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
    padding: '16px 16px 12px',
    display: 'flex',
    flexDirection: 'column',
    minHeight: 0,
  },
  headerRow: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    marginBottom: '12px',
  },
  title: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
    textTransform: 'uppercase',
  },
  subtitle: {
    fontSize: '10px',
    color: '#333',
    letterSpacing: 0,
    textTransform: 'none',
  },
  legend: {
    display: 'flex',
    gap: '12px',
  },
  legendItem: {
    display: 'flex',
    alignItems: 'center',
    gap: '4px',
    fontSize: '10px',
    color: '#444',
    letterSpacing: '1px',
  },
  legendDot: {
    width: '6px',
    height: '6px',
    borderRadius: '50%',
    flexShrink: 0,
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
