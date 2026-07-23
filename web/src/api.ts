// Movie mirrors server.Movie's JSON shape (see server/jellyfin.go).
export interface Movie {
  id: string
  title: string
  year: number
  genres: string[]
  overview: string
  runtime: number
  communityRating: number
  officialRating: string
  posterURL: string
}

// PreviewFilters mirrors the query params server.ParseFilters understands
// (see server/filters.go).
export interface PreviewFilters {
  genres?: string[]
  yearMin?: number
  yearMax?: number
  ratingMin?: number
  officialRatings?: string[]
  runtimeMax?: number
  unwatched?: boolean
  libraryId?: string
  maxMovies?: number
}

export interface PreviewResponse {
  count: number
  movies: Movie[]
}

function buildPreviewParams(filters: PreviewFilters): URLSearchParams {
  const params = new URLSearchParams()
  for (const genre of filters.genres ?? []) {
    params.append('genres', genre)
  }
  if (filters.yearMin) params.set('yearMin', String(filters.yearMin))
  if (filters.yearMax) params.set('yearMax', String(filters.yearMax))
  if (filters.ratingMin) params.set('ratingMin', String(filters.ratingMin))
  for (const rating of filters.officialRatings ?? []) {
    params.append('officialRatings', rating)
  }
  if (filters.runtimeMax) params.set('runtimeMax', String(filters.runtimeMax))
  if (filters.unwatched) params.set('unwatched', 'true')
  if (filters.libraryId) params.set('libraryId', filters.libraryId)
  if (filters.maxMovies) params.set('maxMovies', String(filters.maxMovies))
  return params
}

// getPreview asks the server to query Jellyfin with the given filters and
// returns the matching count plus a capped list of movies for thumbnails.
export async function getPreview(filters: PreviewFilters): Promise<PreviewResponse> {
  const params = buildPreviewParams(filters)
  const res = await fetch(`/api/library/preview?${params.toString()}`)
  if (!res.ok) {
    throw new Error(`preview request failed: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<PreviewResponse>
}

export interface AvailableFilters {
  genres: string[]
  officialRatings: string[]
}

export async function getAvailableFilters(): Promise<AvailableFilters> {
  const res = await fetch('/api/library/filters')
  if (!res.ok) {
    throw new Error(`filters request failed: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<AvailableFilters>
}

// CreateSessionResponse mirrors server.createSessionResponse (see
// server/session.go).
export interface CreateSessionResponse {
  code: string
  joinURL: string
  participantId: string
  token: string
}

// createSession starts a new session with the given admin name. The admin
// becomes participant #1; the caller is responsible for persisting the
// returned token (see SessionSocket.setToken in ws.ts) before connecting.
export async function createSession(adminName: string): Promise<CreateSessionResponse> {
  const res = await fetch('/api/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ adminName }),
  })
  if (!res.ok) {
    throw new Error(`create session failed: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<CreateSessionResponse>
}
