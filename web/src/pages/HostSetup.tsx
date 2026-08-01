import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { getAvailableFilters, getPreview, warmLibrary, type AvailableFilters, type PreviewFilters, type PreviewResponse, type SourceID } from '../api'
import { useSessionStore } from '../store'
import '../styles/admin.css'

const RATING_ORDER: Record<string, number> = {
    'G': 10,
    'TV-Y': 11,
    'TV-Y7': 12,
    'TV-G': 13,
    'PG': 20,
    'TV-PG': 21,
    'PG-13': 30,
    'TV-14': 31,
    'R': 40,
    'TV-MA': 41,
    'NC-17': 50,
    'X': 51,
    'NR': 99,
    'UR': 100,
}

function sortRatings(ratings: string[]): string[] {
    return [...ratings].sort((a, b) => {
        const valA = RATING_ORDER[a.toUpperCase()] ?? 1000
        const valB = RATING_ORDER[b.toUpperCase()] ?? 1000
        if (valA !== valB) return valA - valB
        return a.localeCompare(b)
    })
}

function sortGenres(genres: string[]): string[] {
    return [...genres].sort((a, b) => a.localeCompare(b))
}

// SELECTABLE_SOURCES is the fixed set a host can draw from. Streaming sources
// are offered unconditionally; the server skips any that are not configured on
// this deployment and reports them as unavailable at start.
const SELECTABLE_SOURCES: { id: SourceID; label: string }[] = [
    { id: 'jellyfin', label: 'Jellyfin' },
    { id: 'netflix', label: 'Netflix' },
    { id: 'prime', label: 'Prime Video' },
    { id: 'disney', label: 'Disney+' },
]

// HostSetup is the host's filter + library-preview page. The host arrives
// here with a session already created (the code comes from the ?code= query
// param), picks filters, previews the matching library, and proceeds to the
// lobby.
export default function HostSetup() {
    const [searchParams] = useSearchParams()
    const sessionCode = searchParams.get('code')
    const setFilters = useSessionStore((s) => s.setFilters)

    const [genres, setGenres] = useState<string[]>([])
    const [yearMin, setYearMin] = useState('')
    const [yearMax, setYearMax] = useState('')
    const [officialRatings, setOfficialRatings] = useState<string[]>([])
    const [unwatched, setUnwatched] = useState(false)
    const [sources, setSources] = useState<SourceID[]>(['jellyfin'])
    const [preview, setPreview] = useState<PreviewResponse | null>(null)
    const [available, setAvailable] = useState<AvailableFilters | null>(null)
    // Distinguishes "no answer yet" from "answered, and the answer was an
    // error". Keying availability off `available === null` alone conflates the
    // two and leaves every source selectable forever when the fetch fails.
    const [sourcesLoaded, setSourcesLoaded] = useState(false)
    const [loading, setLoading] = useState(false)
    const [error, setError] = useState<string | null>(null)

    useEffect(() => {
        getAvailableFilters()
            .then((f) => {
                const configured = f.sources ?? []
                setAvailable({
                    genres: sortGenres(f.genres),
                    officialRatings: sortRatings(f.officialRatings),
                    sources: configured,
                })
                // Reconcile any selection made while the answer was in flight.
                // A source the server cannot query must not survive in state:
                // it would ship in host:start and be dropped silently, and its
                // chip would render checked-and-disabled, which cannot be
                // cleared because a disabled input fires no onChange.
                setSources((current) => {
                    const allowed = current.filter((id) => configured.includes(id))
                    return allowed.length > 0 ? allowed : ['jellyfin']
                })
                setSourcesLoaded(true)
            })
            .catch((err) => {
                console.error('Failed to load available filters:', err)
                setError('Could not load filter options from Jellyfin.')
                // The question has been answered, badly. Fall back to Jellyfin
                // rather than leaving every source selectable on no information.
                setSources(['jellyfin'])
                setSourcesLoaded(true)
            })
    }, [])

    function toggleGenre(genre: string) {
        setGenres((prev) => (prev.includes(genre) ? prev.filter((g) => g !== genre) : [...prev, genre]))
    }

    function toggleRating(rating: string) {
        setOfficialRatings((prev) => (prev.includes(rating) ? prev.filter((r) => r !== rating) : [...prev, rating]))
    }

    // Three states, three answers:
    //   loading           -> everything enabled; nothing has been claimed yet
    //   loaded, succeeded -> only what the server reported
    //   loaded, failed    -> Jellyfin only, which the server always configures
    function sourceConfigured(id: SourceID): boolean {
        if (!sourcesLoaded) return true
        if (!available) return id === 'jellyfin'
        return available.sources.includes(id)
    }

    // At least one source must stay selected: a session with no source has no
    // deck to deal.
    function toggleSource(id: SourceID) {
        if (!sourceConfigured(id)) return
        setSources((current) => {
            if (!current.includes(id)) return [...current, id]
            if (current.length === 1) return current
            return current.filter((s) => s !== id)
        })
    }

    function currentFilters(): PreviewFilters {
        return {
            sources,
            genres,
            yearMin: yearMin ? Number(yearMin) : undefined,
            yearMax: yearMax ? Number(yearMax) : undefined,
            officialRatings: officialRatings.length > 0 ? officialRatings : undefined,
            unwatched: sources.includes('jellyfin') ? unwatched : false,
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
    // host:start. Filters live in the shared session store rather than the
    // URL since they can include an arbitrary list of genres.
    function handleGoToLobby() {
        setFilters(currentFilters())
        // Warm the poster cache during the lobby-fill window (fire-and-forget;
        // must never block entering the lobby).
        warmLibrary(currentFilters()).catch((err) =>
            console.error('Failed to warm poster cache:', err),
        )
    }

    return (
        <div className="admin-setup">
            <h1>Start a Showdown</h1>

            {sessionCode && (
                <p className="admin-session-banner">
                    Session <strong>{sessionCode}</strong> created
                </p>
            )}

            <fieldset className="chip-group source-picker">
                <legend>Sources</legend>
                {SELECTABLE_SOURCES.map((s) => {
                    const configured = sourceConfigured(s.id)
                    return (
                        <label
                            key={s.id}
                            className={`chip source-chip source-chip-${s.id} ${sources.includes(s.id) ? 'checked' : ''}${configured ? '' : ' disabled'}`}
                            title={configured ? undefined : 'Not configured on this server'}
                        >
                            <input
                                type="checkbox"
                                className="sr-only"
                                checked={sources.includes(s.id)}
                                disabled={!configured}
                                onChange={() => toggleSource(s.id)}
                            />
                            {s.label}
                            {!configured && <span className="chip-hint"> (not configured)</span>}
                        </label>
                    )
                })}
            </fieldset>

            {available && available.genres.length > 0 && (
                <fieldset className="chip-group">
                    <legend>Genres</legend>
                    {available.genres.map((genre) => (
                        <label key={genre} className={`chip ${genres.includes(genre) ? 'checked' : ''}`}>
                            <input
                                type="checkbox"
                                className="sr-only"
                                checked={genres.includes(genre)}
                                onChange={() => toggleGenre(genre)}
                            />
                            {genre}
                        </label>
                    ))}
                </fieldset>
            )}

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

            {available && available.officialRatings.length > 0 && (
                <fieldset className="chip-group">
                    <legend>Parental Rating</legend>
                    {available.officialRatings.map((rating) => (
                        <label key={rating} className={`chip ${officialRatings.includes(rating) ? 'checked' : ''}`}>
                            <input
                                type="checkbox"
                                className="sr-only"
                                checked={officialRatings.includes(rating)}
                                onChange={() => toggleRating(rating)}
                            />
                            {rating}
                        </label>
                    ))}
                </fieldset>
            )}

            <label className={`unwatched-toggle${sources.includes('jellyfin') ? '' : ' disabled'}`}>
                <input
                    type="checkbox"
                    checked={unwatched && sources.includes('jellyfin')}
                    disabled={!sources.includes('jellyfin')}
                    onChange={(e) => setUnwatched(e.target.checked)}
                />
                Unwatched only
                {!sources.includes('jellyfin') && <span className="hint"> (Jellyfin only)</span>}
            </label>

            {sessionCode && (
                <Link to={`/join/${sessionCode}`} onClick={handleGoToLobby} className="btn btn-primary">
                    Go to the Lobby →
                </Link>
            )}

            <button type="button" onClick={handlePreview} disabled={loading}>
                {loading ? 'Loading…' : 'Preview'}
            </button>

            {error && <p className="preview-error">{error}</p>}

            {preview && (
                <div className="preview-results">
                    {(preview.unavailable?.length ?? 0) > 0 && (
                        <p className="preview-unavailable">
                            Could not reach: {preview.unavailable.join(', ')}. Those results are missing.
                        </p>
                    )}
                    <p className="preview-count">
                        {preview.count >= 150 ? `showing ${preview.count} of many` : `${preview.count} movies match`}
                    </p>
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
