import React, { useMemo } from 'react'
import { formatTime } from '../types'

interface Props {
  elapsedSec: number
  roundDuration: number
  paused: boolean
}

export function RoundTimer({ elapsedSec, roundDuration, paused }: Props) {
  const { currentRound, roundElapsed, roundRemaining } = useMemo(() => {
    const currentRound = Math.floor(elapsedSec / roundDuration) + 1
    const roundElapsed = elapsedSec % roundDuration
    const roundRemaining = roundDuration - roundElapsed
    return { currentRound, roundElapsed, roundRemaining }
  }, [elapsedSec, roundDuration])

  // Progress percentage for the round
  const progress = (roundElapsed / roundDuration) * 100

  return (
    <div style={styles.container}>
      <div style={styles.roundLabel}>
        ROUND {currentRound}
        {paused && <span style={styles.pausedBadge}>PAUSED</span>}
      </div>
      
      <div style={styles.timeDisplay}>
        {formatTime(roundRemaining)}
      </div>
      
      <div style={styles.progressBar}>
        <div 
          style={{
            ...styles.progressFill,
            width: `${progress}%`,
          }}
        />
      </div>
      
      <div style={styles.totalTime}>
        Total: {formatTime(elapsedSec)}
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  container: {
    textAlign: 'center',
    padding: '20px',
  },
  roundLabel: {
    fontSize: '14px',
    fontWeight: 600,
    letterSpacing: '4px',
    color: '#888',
    marginBottom: '8px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: '12px',
  },
  pausedBadge: {
    fontSize: '10px',
    letterSpacing: '2px',
    color: '#f59e0b',
    background: '#f59e0b22',
    padding: '4px 8px',
    borderRadius: '4px',
  },
  timeDisplay: {
    fontSize: '72px',
    fontWeight: 700,
    color: '#f0f0f0',
    fontVariantNumeric: 'tabular-nums',
    lineHeight: 1,
    marginBottom: '16px',
  },
  progressBar: {
    height: '4px',
    background: '#1e1e1e',
    borderRadius: '2px',
    overflow: 'hidden',
    maxWidth: '300px',
    margin: '0 auto 12px',
  },
  progressFill: {
    height: '100%',
    background: '#ff4d4d',
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
  totalTime: {
    fontSize: '12px',
    color: '#444',
    letterSpacing: '1px',
  },
}
