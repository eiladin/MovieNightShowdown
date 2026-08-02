import { forwardRef, useImperativeHandle } from 'react'
import { motion, useAnimationControls, useMotionValue, useTransform, type PanInfo } from 'motion/react'
import type { Movie } from '../api'
import SourceBadges from './SourceBadges'

export type SwipeDirection = 'left' | 'right' | 'up' | 'down'

// SwipeCardApi is the imperative handle Swipe.tsx drives: the yes/no buttons
// call swipe(), and Undo calls restoreCard().
export interface SwipeCardApi {
  swipe(dir?: SwipeDirection): Promise<void>
  restoreCard(): Promise<void>
}

interface CardProps {
  movie: Movie
  active: boolean
  visible?: boolean
  onSwipe: (dir: SwipeDirection) => void
}

// Horizontal displacement, in pixels, that commits a drag to a swipe. Below it
// the card snaps back to centre.
const SWIPE_THRESHOLD = 65

// How far off-screen a committed card flies. Wide enough to clear any viewport
// this app is used on.
const FLY_AWAY_X = 1000

const FLY_TRANSITION = { duration: 0.3, ease: 'easeOut' } as const

// Card renders one movie poster as a draggable card. onSwipe fires when the
// card is dragged past SWIPE_THRESHOLD or swiped programmatically through the
// imperative handle: 'right' = yes, 'left' = no (see Swipe.tsx, which sends the
// corresponding `swipe` WS message). Dragging is constrained to the x axis, so
// up and down swipes cannot occur.
const Card = forwardRef<SwipeCardApi, CardProps>(function Card({ movie, active, visible, onSwipe }, ref) {
  const controls = useAnimationControls()
  const x = useMotionValue(0)
  // A tilt in the direction of travel, matching the feel of the card stack this
  // component replaced.
  const rotate = useTransform(x, [-FLY_AWAY_X, 0, FLY_AWAY_X], [-30, 0, 30])

  // flyAway notifies the parent first and animates afterwards, so the vote is
  // recorded and the next card becomes active without waiting on the
  // animation. The returned promise resolves once the card has left.
  async function flyAway(dir: SwipeDirection) {
    onSwipe(dir)
    await controls.start({
      x: dir === 'left' ? -FLY_AWAY_X : FLY_AWAY_X,
      opacity: 0,
      transition: FLY_TRANSITION,
    })
  }

  useImperativeHandle(ref, () => ({
    swipe: (dir: SwipeDirection = 'right') => flyAway(dir),
    restoreCard: async () => {
      await controls.start({ x: 0, opacity: 1, transition: FLY_TRANSITION })
    },
  }))

  function handleDragEnd(_event: unknown, info: PanInfo) {
    if (info.offset.x > SWIPE_THRESHOLD) {
      void flyAway('right')
    } else if (info.offset.x < -SWIPE_THRESHOLD) {
      void flyAway('left')
    }
    // Below the threshold the drag constraints pull the card back to centre on
    // their own, so there is nothing to do here.
  }

  return (
    <motion.div
      className={`swipe-card${active ? ' swipe-card-active' : ''}`}
      style={{ x, rotate }}
      drag="x"
      dragConstraints={{ left: 0, right: 0 }}
      dragElastic={1}
      dragMomentum={false}
      onDragEnd={handleDragEnd}
      animate={controls}
    >
      {visible !== false && (
        <div className="swipe-card-inner">
          <img className="swipe-card-poster" src={movie.posterURL} alt={movie.title} draggable={false} />
          <div className="swipe-card-meta">
            <h2>
              {movie.title} <span className="swipe-card-year">({movie.year})</span>
            </h2>
            <p className="swipe-card-genres">{movie.genres.join(', ')}</p>
            <p className="swipe-card-runtime">{movie.runtime} min</p>
            <SourceBadges availability={movie.availability} />
          </div>
        </div>
      )}
    </motion.div>
  )
})

export default Card
