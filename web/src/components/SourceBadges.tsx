import type { Availability, SourceID } from '../api'

// SOURCE_LABELS is the display name for each source. Keys mirror
// server.SourceID (see server/source.go).
const SOURCE_LABELS: Record<SourceID, string> = {
    jellyfin: 'Jellyfin',
    netflix: 'Netflix',
    prime: 'Prime Video',
    disney: 'Disney+',
}

interface SourceBadgesProps {
    availability: Availability[] | undefined
    className?: string
}

// SourceBadges renders one badge per service carrying this movie. A film in the
// local library and on a streaming service shows both: knowing a local copy
// exists changes the decision, because it starts instantly and has no ads.
export default function SourceBadges({ availability, className }: SourceBadgesProps) {
    if (!availability || availability.length === 0) return null
    return (
        <ul className={`source-badges${className ? ' ' + className : ''}`}>
            {availability.map((a) => (
                <li key={a.source} className={`source-badge source-badge-${a.source}`}>
                    {SOURCE_LABELS[a.source] ?? a.source}
                </li>
            ))}
        </ul>
    )
}
