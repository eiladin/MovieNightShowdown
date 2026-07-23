import { Link } from 'react-router-dom'

// Landing is a placeholder entry page. Session creation and the join flow
// land here in Phase 3 (see docs/TASKS.md 3.5).
export default function Landing() {
  return (
    <div className="landing">
      <h1>Movie Night Showdown</h1>
      <p>Coming soon</p>
      <Link to="/admin">Start a Showdown</Link>
    </div>
  )
}
