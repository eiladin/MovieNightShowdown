import { createRef, useEffect, useMemo, useRef, useState } from 'react'
import Card, { type SwipeDirection, type TinderCardApi } from '../components/Card'
import { useSessionStore, type Vote } from '../store'
import type { MatchPayload, ProgressPayload, SessionSocket } from '../ws'
import '../styles/swipe.css'

interface SwipeProps {
  socket: SessionSocket
}

// Swipe is the swipe-deck screen (Phase 4, task 4.3): a stack of Cards over
// the session's dealt deck, yes/no/undo controls, and a HUD driven by the
// server's `progress` broadcasts. It never shows another participant's
// individual votes — only aggregate counts.
export default function Swipe({ socket }: SwipeProps) {
  const deck = useSessionStore((s) => s.deck)
  const requiredCount = useSessionStore((s) => s.requiredCount)
  const recordVote = useSessionStore((s) => s.recordVote)
  const clearVote = useSessionStore((s) => s.clearVote)
  const setStatus = useSessionStore((s) => s.setStatus)

  const [progress, setProgress] = useState<ProgressPayload | null>(null)
  // matchedMovie is only tracked here so the swipe screen can stop accepting
  // input the instant a match fires; the full reveal + confetti UI is built
  // in Phase 5 (Result.tsx / Confetti.tsx).
  const [matchedMovie, setMatchedMovie] = useState<MatchPayload['movie'] | null>(null)

  const childRefs = useMemo(() => deck.map(() => createRef<TinderCardApi>()), [deck])

  const [currentIndex, setCurrentIndex] = useState(deck.length - 1)
  const currentIndexRef = useRef(currentIndex)
  // lastSwipe tracks this device's own most recent vote, so Undo knows which
  // card to restore and which local vote to clear. The server independently
  // tracks the authoritative LastSwipe for the actual undo semantics.
  const lastSwipeRef = useRef<{ movieId: string; vote: Vote } | null>(null)

  useEffect(() => {
    setCurrentIndex(deck.length - 1)
    currentIndexRef.current = deck.length - 1
    lastSwipeRef.current = null
  }, [deck])

  useEffect(() => {
    const offProgress = socket.on('progress', (payload) => setProgress(payload as ProgressPayload))
    const offMatch = socket.on('match', (payload) => {
      const { movie } = payload as MatchPayload
      setMatchedMovie(movie)
      setStatus('matched')
    })
    return () => {
      offProgress()
      offMatch()
    }
  }, [socket, setStatus])

  function updateCurrentIndex(next: number) {
    setCurrentIndex(next)
    currentIndexRef.current = next
  }

  function handleCardSwiped(dir: SwipeDirection, movieId: string, index: number) {
    if (dir !== 'left' && dir !== 'right') return
    const vote: Vote = dir === 'right' ? 'yes' : 'no'

    recordVote(movieId, vote)
    lastSwipeRef.current = { movieId, vote }
    socket.send('swipe', { movieID: movieId, dir: vote })

    if (currentIndexRef.current === index) {
      updateCurrentIndex(index - 1)
    }
  }

  function swipeTop(dir: SwipeDirection) {
    const index = currentIndexRef.current
    if (index < 0) return
    childRefs[index]?.current?.swipe(dir)
  }

  function handleUndo() {
    const last = lastSwipeRef.current
    if (!last) return

    const newIndex = currentIndexRef.current + 1
    if (newIndex >= deck.length) return

    updateCurrentIndex(newIndex)
    childRefs[newIndex]?.current?.restoreCard()
    clearVote(last.movieId)
    lastSwipeRef.current = null
    socket.send('undo', {})
  }

  const canSwipe = currentIndex >= 0
  const canUndo = lastSwipeRef.current !== null && currentIndex < deck.length - 1

  if (matchedMovie) {
    // Placeholder only: the confetti/full-screen reveal is Phase 5 scope.
    return (
      <div className="swipe-screen swipe-matched">
        <h1>It&apos;s a match!</h1>
        <p>{matchedMovie.title}</p>
      </div>
    )
  }

  return (
    <div className="swipe-screen">
      <div className="swipe-hud">
        {progress
          ? `${progress.participantsSwiped} of ${progress.participantsTotal} swiped this card · ${progress.cardsRemaining} cards left`
          : `0 of ${requiredCount} swiped · ${deck.length} cards left`}
      </div>

      <div className="swipe-deck">
        {deck.length === 0 && <p className="swipe-empty">Waiting for the deck…</p>}
        {deck.map((movie, index) => (
          <Card
            key={movie.id}
            ref={childRefs[index]}
            movie={movie}
            active={index === currentIndex}
            onSwipe={(dir) => handleCardSwiped(dir, movie.id, index)}
          />
        ))}
      </div>

      <div className="swipe-controls">
        <button type="button" className="swipe-btn swipe-btn-no" onClick={() => swipeTop('left')} disabled={!canSwipe}>
          No
        </button>
        <button type="button" className="swipe-btn swipe-btn-undo" onClick={handleUndo} disabled={!canUndo}>
          Undo
        </button>
        <button
          type="button"
          className="swipe-btn swipe-btn-yes"
          onClick={() => swipeTop('right')}
          disabled={!canSwipe}
        >
          Yes
        </button>
      </div>
    </div>
  )
}
