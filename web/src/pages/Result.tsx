import { useSessionStore } from '../store'
import Confetti from '../components/Confetti'
import '../styles/result.css'

export default function Result() {
  const status = useSessionStore((s) => s.status)
  const winner = useSessionStore((s) => s.winner)

  if (status === 'matched' && winner) {
    return (
      <div className="result-screen">
        <Confetti />
        <h1>It&apos;s a match!</h1>
        <div className="winner-card">
          <img src={winner.posterURL} alt={winner.title} />
          <div className="winner-meta">
            <h2>{winner.title}</h2>
            <p>{winner.year} · {winner.genres?.join(', ')}</p>
          </div>
        </div>
      </div>
    )
  }

  return null
}
