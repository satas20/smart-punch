import React from 'react'
import { HandState, estimateBatteryLife } from '../types'

interface Props {
  label: 'LEFT' | 'RIGHT'
  hand: HandState
}

export function GloveCard({ label, hand }: Props) {
  const isLeft = label === 'LEFT'
  
  // Status determination
  let statusColor = '#333'
  let statusText = 'Disconnected'
  let statusSubtext = 'Waiting for glove...'
  
  if (hand.connected) {
    if (hand.calibrated) {
      statusColor = '#22c55e'
      statusText = 'Ready'
      statusSubtext = 'Calibrated and connected'
    } else {
      statusColor = '#f59e0b'
      statusText = 'Calibrating...'
      statusSubtext = 'Hold glove still for 3 seconds'
    }
  }

  return (
    <div style={{
      ...styles.card,
      borderColor: hand.connected ? (hand.calibrated ? '#22c55e33' : '#f59e0b33') : '#1e1e1e',
    }}>
      {/* Header */}
      <div style={styles.header}>
        <div style={styles.labelContainer}>
          <span style={{
            ...styles.dot,
            background: statusColor,
            animation: hand.connected && !hand.calibrated ? 'pulse 1s ease-in-out infinite' : 'none',
          }} />
          <span style={styles.label}>{isLeft ? 'LEFT GLOVE' : 'RIGHT GLOVE'}</span>
        </div>
        <span style={styles.deviceName}>FighterLink_{isLeft ? 'L' : 'R'}</span>
      </div>

      {/* Status */}
      <div style={styles.statusSection}>
        <span style={{ ...styles.statusText, color: statusColor }}>{statusText}</span>
        <span style={styles.statusSubtext}>{statusSubtext}</span>
      </div>

      {/* Battery */}
      {hand.connected && (
        <div style={styles.batterySection}>
          <div style={styles.batteryRow}>
            <span style={styles.batteryIcon}>🔋</span>
            <span style={styles.batteryPercent}>{hand.battery}%</span>
            <span style={styles.batteryTime}>~{estimateBatteryLife(hand.battery)}</span>
          </div>
          <div style={styles.batteryBar}>
            <div style={{
              ...styles.batteryFill,
              width: `${hand.battery}%`,
              background: hand.battery > 20 ? '#22c55e' : '#ef4444',
            }} />
          </div>
        </div>
      )}

      {/* Sensor Data (only when connected) */}
      {hand.connected && (
        <div style={styles.sensorSection}>
          <div style={styles.sensorRow}>
            <span style={styles.sensorLabel}>ACCEL</span>
            <span style={styles.sensorValue}>
              {hand.current_accel[0].toFixed(1)}, {hand.current_accel[1].toFixed(1)}, {hand.current_accel[2].toFixed(1)}
              <span style={styles.sensorUnit}> m/s²</span>
            </span>
          </div>
          <div style={styles.sensorRow}>
            <span style={styles.sensorLabel}>GYRO</span>
            <span style={styles.sensorValue}>
              {hand.current_gyro[0].toFixed(1)}, {hand.current_gyro[1].toFixed(1)}, {hand.current_gyro[2].toFixed(1)}
              <span style={styles.sensorUnit}> °/s</span>
            </span>
          </div>
        </div>
      )}
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
  card: {
    background: '#111',
    border: '1px solid #1e1e1e',
    borderRadius: '12px',
    padding: '20px',
    minWidth: '280px',
    flex: 1,
  },
  header: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '16px',
  },
  labelContainer: {
    display: 'flex',
    alignItems: 'center',
    gap: '10px',
  },
  dot: {
    width: '10px',
    height: '10px',
    borderRadius: '50%',
    flexShrink: 0,
  },
  label: {
    fontSize: '14px',
    fontWeight: 700,
    letterSpacing: '2px',
    color: '#f0f0f0',
  },
  deviceName: {
    fontSize: '11px',
    color: '#444',
    fontFamily: 'monospace',
  },
  statusSection: {
    marginBottom: '16px',
  },
  statusText: {
    fontSize: '18px',
    fontWeight: 600,
    display: 'block',
    marginBottom: '4px',
  },
  statusSubtext: {
    fontSize: '12px',
    color: '#555',
  },
  batterySection: {
    marginBottom: '16px',
    padding: '12px',
    background: '#0a0a0a',
    borderRadius: '8px',
  },
  batteryRow: {
    display: 'flex',
    alignItems: 'center',
    gap: '8px',
    marginBottom: '8px',
  },
  batteryIcon: {
    fontSize: '14px',
  },
  batteryPercent: {
    fontSize: '16px',
    fontWeight: 600,
    color: '#f0f0f0',
  },
  batteryTime: {
    fontSize: '12px',
    color: '#555',
    marginLeft: 'auto',
  },
  batteryBar: {
    height: '4px',
    background: '#1e1e1e',
    borderRadius: '2px',
    overflow: 'hidden',
  },
  batteryFill: {
    height: '100%',
    borderRadius: '2px',
    transition: 'width 0.3s ease',
  },
  sensorSection: {
    padding: '12px',
    background: '#0a0a0a',
    borderRadius: '8px',
  },
  sensorRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginBottom: '6px',
  },
  sensorLabel: {
    fontSize: '10px',
    fontWeight: 600,
    letterSpacing: '1px',
    color: '#555',
  },
  sensorValue: {
    fontSize: '12px',
    fontFamily: 'monospace',
    color: '#888',
  },
  sensorUnit: {
    fontSize: '10px',
    color: '#444',
  },
}
