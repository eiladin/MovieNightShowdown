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
  officialRating?: string
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
  if (filters.officialRating) params.set('officialRating', filters.officialRating)
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
