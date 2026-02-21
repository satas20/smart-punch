import { useBoxingSocket } from './hooks/useBoxingSocket'
import { PreTrainingView } from './components/PreTrainingView'
import { LiveTrainingView } from './components/LiveTrainingView'
import { PostTrainingView } from './components/PostTrainingView'

export default function App() {
  const {
    state,
    connected,
    phase,
    roundDuration,
    setRoundDuration,
    finalState,
    startSession,
    pauseSession,
    resumeSession,
    stopSession,
    resetSession,
  } = useBoxingSocket()

  // Render based on current phase
  switch (phase) {
    case 'pre':
      return (
        <PreTrainingView
          state={state}
          connected={connected}
          roundDuration={roundDuration}
          setRoundDuration={setRoundDuration}
          onStart={startSession}
        />
      )

    case 'live':
      return (
        <LiveTrainingView
          state={state}
          roundDuration={roundDuration}
          onPause={pauseSession}
          onResume={resumeSession}
          onStop={stopSession}
        />
      )

    case 'post':
      // Use finalState if available (snapshot at stop), otherwise use current state
      const displayState = finalState || state
      return (
        <PostTrainingView
          state={displayState}
          onNewSession={resetSession}
        />
      )

    default:
      return null
  }
}
