interface Props {
  avgForce: number
  maxForce: number
  ppm: number
  elapsedSec: number
}

function formatTime(sec: number): string {
  const m = Math.floor(sec / 60).toString().padStart(2, '0')
  const s = Math.floor(sec % 60).toString().padStart(2, '0')
  return `${m}:${s}`
}

function StatCard({ label, value, unit }: { label: string; value: string; unit?: string }) {
  return (
    <div style={styles.card}>
      <p style={styles.cardLabel}>{label}</p>
      <p style={styles.cardValue}>
        {value}
        {unit && <span style={styles.cardUnit}> {unit}</span>}
      </p>
    </div>
  )
}

export function StatsBar({ avgForce, maxForce, ppm, elapsedSec }: Props) {
  return (
    <div style={styles.bar}>
      <StatCard label="AVG FORCE" value={avgForce.toFixed(1)} unit="m/s²" />
      <StatCard label="MAX FORCE" value={maxForce.toFixed(1)} unit="m/s²" />
      <StatCard label="PUNCH/MIN" value={ppm.toFixed(1)} />
      <StatCard label="SESSION" value={formatTime(elapsedSec)} />
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  bar: {
    display: 'flex',
    flexDirection: 'column',
    gap: '1px',
    background: '#111',
    borderRight: '1px solid #1e1e1e',
    minWidth: '140px',
  },
  card: {
    padding: '20px 16px',
    borderBottom: '1px solid #1e1e1e',
    flex: 1,
  },
  cardLabel: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#555',
    marginBottom: '6px',
    textTransform: 'uppercase',
  },
  cardValue: {
    fontSize: '22px',
    fontWeight: 600,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
  },
  cardUnit: {
    fontSize: '11px',
    color: '#666',
    fontWeight: 400,
  },
}
