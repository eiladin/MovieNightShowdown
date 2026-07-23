import { forwardRef } from 'react'
import TinderCard from 'react-tinder-card'
import type { Movie } from '../api'

export type SwipeDirection = 'left' | 'right' | 'up' | 'down'

// TinderCardApi is the subset of react-tinder-card's imperative API this app
// uses (see node_modules/react-tinder-card/index.d.ts).
export interface TinderCardApi {
  swipe(dir?: SwipeDirection): Promise<void>
  restoreCard(): Promise<void>
}

interface CardProps {
  movie: Movie
  active: boolean
  visible?: boolean
  onSwipe: (dir: SwipeDirection) => void
}

// Card wraps one react-tinder-card around a movie poster. onSwipe fires when
// the card is dragged past its threshold or programmatically swiped:
// 'right' = yes, 'left' = no (see Swipe.tsx, which sends the corresponding
// `swipe` WS message).
const Card = forwardRef<TinderCardApi, CardProps>(function Card({ movie, active, visible, onSwipe }, ref) {
  return (
    <TinderCard
      ref={ref}
      className={`swipe-card${active ? ' swipe-card-active' : ''}`}
      onSwipe={onSwipe}
      preventSwipe={['up', 'down']}
      swipeRequirementType="position"
      swipeThreshold={65}
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
          </div>
        </div>
      )}
    </TinderCard>
  )
})

export default Card
