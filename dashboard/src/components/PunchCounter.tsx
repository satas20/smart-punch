import { useEffect, useRef } from 'react'
import { TYPE_COLORS } from './PunchBreakdown'

interface Props {
  count:     number
  active:    boolean
  lastType?: string  // most recent punch type
}

export function PunchCounter({ count, active, lastType }: Props) {
  const prevCount = useRef(count)
  const flashRef  = useRef<HTMLDivElement>(null)
  const typeColor = TYPE_COLORS[lastType ?? ''] ?? '#ff4d4d'

  // Flash animation on new punch
  useEffect(() => {
    if (count > prevCount.current && flashRef.current) {
      const el = flashRef.current
      el.style.transition = 'none'
      el.style.color = typeColor
      el.style.textShadow = `0 0 40px ${typeColor}`
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          el.style.transition = 'color 0.4s ease, text-shadow 0.4s ease'
          el.style.color = '#ffffff'
          el.style.textShadow = '0 0 20px rgba(255,255,255,0.15)'
        })
      })
    }
    prevCount.current = count
  }, [count, typeColor])

  return (
    <div style={styles.wrapper}>
      <p style={styles.label}>PUNCH COUNT</p>

      <div ref={flashRef} style={styles.number}>
        {count}
      </div>

      {/* Last punch type badge */}
      {lastType ? (
        <p style={{ ...styles.typeLabel, color: typeColor }}>
          {lastType.toUpperCase()}
        </p>
      ) : (
        !active && <p style={styles.hint}>Press START to begin a session</p>
      )}
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  wrapper: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    padding: '28px 16px',
    borderBottom: '1px solid #1e1e1e',
  },
  label: {
    fontSize: '12px',
    letterSpacing: '4px',
    color: '#555',
    marginBottom: '8px',
    textTransform: 'uppercase',
  },
  number: {
    fontSize: 'clamp(72px, 14vw, 144px)',
    fontWeight: 700,
    lineHeight: 1,
    color: '#ffffff',
    textShadow: '0 0 20px rgba(255,255,255,0.15)',
    fontVariantNumeric: 'tabular-nums',
    transition: 'color 0.4s ease, text-shadow 0.4s ease',
  },
  typeLabel: {
    marginTop: '10px',
    fontSize: '14px',
    fontWeight: 600,
    letterSpacing: '4px',
    transition: 'color 0.3s ease',
  },
  hint: {
    marginTop: '16px',
    fontSize: '13px',
    color: '#444',
  },
}
