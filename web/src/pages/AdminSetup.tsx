import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { getPreview, type PreviewFilters, type PreviewResponse } from '../api'
import { useSessionStore } from '../store'
import '../styles/admin.css'

const GENRE_OPTIONS = [
  'Action',
  'Adventure',
  'Animation',
  'Comedy',
  'Crime',
  'Documentary',
  'Drama',
  'Family',
  'Fantasy',
  'History',
  'Horror',
  'Music',
  'Mystery',
  'Romance',
  'Science Fiction',
  'Thriller',
  'War',
  'Western',
]

// AdminSetup is the minimal filter + preview page for Phase 2. Session
// creation ("Begin") is wired up in Phase 4.
export default function AdminSetup() {
  const [searchParams] = useSearchParams()
  const sessionCode = searchParams.get('code')
  const setFilters = useSessionStore((s) => s.setFilters)

  const [genres, setGenres] = useState<string[]>([])
  const [yearMin, setYearMin] = useState('')
  const [yearMax, setYearMax] = useState('')
  const [officialRating, setOfficialRating] = useState('')
  const [unwatched, setUnwatched] = useState(false)
  const [preview, setPreview] = useState<PreviewResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  function toggleGenre(genre: string) {
    setGenres((prev) => (prev.includes(genre) ? prev.filter((g) => g !== genre) : [...prev, genre]))
  }

  function currentFilters(): PreviewFilters {
    return {
      genres,
      yearMin: yearMin ? Number(yearMin) : undefined,
      yearMax: yearMax ? Number(yearMax) : undefined,
      officialRating: officialRating || undefined,
      unwatched,
    }
  }

  async function handlePreview() {
    setLoading(true)
    setError(null)
    try {
      const result = await getPreview(currentFilters())
      setPreview(result)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Preview failed')
      setPreview(null)
    } finally {
      setLoading(false)
    }
  }

  // Carry the chosen filters over to the Lobby, where "Begin" sends them in
  // admin:start (Phase 4). Filters live in the shared session store rather
  // than the URL since they can include an arbitrary list of genres.
  function handleGoToLobby() {
    setFilters(currentFilters())
  }

  return (
    <div className="admin-setup">
      <h1>Start a Showdown</h1>

      {sessionCode && (
        <p className="admin-session-banner">
          Session <strong>{sessionCode}</strong> created —{' '}
          <Link to={`/join/${sessionCode}`} onClick={handleGoToLobby}>
            go to the Lobby
          </Link>{' '}
          to see who has joined.
        </p>
      )}

      <fieldset className="genre-filter">
        <legend>Genres</legend>
        {GENRE_OPTIONS.map((genre) => (
          <label key={genre} className="genre-option">
            <input type="checkbox" checked={genres.includes(genre)} onChange={() => toggleGenre(genre)} />
            {genre}
          </label>
        ))}
      </fieldset>

      <div className="year-range">
        <label>
          Year from
          <input type="number" value={yearMin} onChange={(e) => setYearMin(e.target.value)} placeholder="1970" />
        </label>
        <label>
          Year to
          <input type="number" value={yearMax} onChange={(e) => setYearMax(e.target.value)} placeholder="2026" />
        </label>
      </div>

      <label className="rating-select">
        Parental Rating
        <select value={officialRating} onChange={(e) => setOfficialRating(e.target.value)}>
          <option value="">Any</option>
          <option value="G">G</option>
          <option value="PG">PG</option>
          <option value="PG-13">PG-13</option>
          <option value="R">R</option>
          <option value="NC-17">NC-17</option>
        </select>
      </label>

      <label className="unwatched-toggle">
        <input type="checkbox" checked={unwatched} onChange={(e) => setUnwatched(e.target.checked)} />
        Unwatched only
      </label>

      <button type="button" onClick={handlePreview} disabled={loading}>
        {loading ? 'Loading…' : 'Preview'}
      </button>

      {error && <p className="preview-error">{error}</p>}

      {preview && (
        <div className="preview-results">
          <p className="preview-count">{preview.count} movies match</p>
          <div className="poster-grid">
            {preview.movies.map((movie) => (
              <img
                key={movie.id}
                src={movie.posterURL}
                alt={movie.title}
                title={`${movie.title} (${movie.year})`}
                loading="lazy"
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
