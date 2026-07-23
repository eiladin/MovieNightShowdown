package server

import (
	"net/url"
	"strconv"
	"strings"
)

// defaultMaxMovies is the deck cap applied when the admin does not set one.
const defaultMaxMovies = 50

// Filters holds the admin's library filter selections, parsed from the query
// params of GET /api/library/preview, and also sent as JSON inside the
// admin:start WS payload (Phase 4) — hence the json tags, matching the
// frontend's PreviewFilters shape (web/src/api.ts).
type Filters struct {
	Genres         []string `json:"genres"`
	YearMin        int      `json:"yearMin"`
	YearMax        int      `json:"yearMax"`
	RatingMin      float64  `json:"ratingMin"`      // minimum CommunityRating
	OfficialRating string   `json:"officialRating"` // MPAA rating, e.g. "PG-13"
	RuntimeMax     int      `json:"runtimeMax"`     // minutes; filtered client-side
	Unwatched      bool     `json:"unwatched"`
	LibraryID      string   `json:"libraryId"`
	MaxMovies      int      `json:"maxMovies"` // deck cap, default 50
}

// ParseFilters parses Filters from request query params.
func ParseFilters(q url.Values) Filters {
	f := Filters{
		Genres:         q["genres"],
		OfficialRating: q.Get("officialRating"),
		Unwatched:      q.Get("unwatched") == "true",
		LibraryID:      q.Get("libraryId"),
		MaxMovies:      defaultMaxMovies,
	}
	if v, err := strconv.Atoi(q.Get("yearMin")); err == nil {
		f.YearMin = v
	}
	if v, err := strconv.Atoi(q.Get("yearMax")); err == nil {
		f.YearMax = v
	}
	if v, err := strconv.ParseFloat(q.Get("ratingMin"), 64); err == nil {
		f.RatingMin = v
	}
	if v, err := strconv.Atoi(q.Get("runtimeMax")); err == nil {
		f.RuntimeMax = v
	}
	if v, err := strconv.Atoi(q.Get("maxMovies")); err == nil && v > 0 {
		f.MaxMovies = v
	}
	return f
}

// apply maps Filters onto a Jellyfin /Items query. hasUserID reports whether
// the client is configured with a JellyfinUserID, required for the
// IsUnplayed filter to mean anything.
func (f Filters) apply(q url.Values, hasUserID bool) {
	if len(f.Genres) > 0 {
		// Jellyfin ORs genres together using a "|" delimiter.
		q.Set("Genres", strings.Join(f.Genres, "|"))
	}
	if f.YearMin > 0 || f.YearMax > 0 {
		q.Set("Years", yearsList(f.YearMin, f.YearMax))
	}
	if f.RatingMin > 0 {
		q.Set("MinCommunityRating", strconv.FormatFloat(f.RatingMin, 'f', -1, 64))
	}
	if f.OfficialRating != "" {
		q.Set("OfficialRatings", f.OfficialRating)
	}
	if f.Unwatched && hasUserID {
		q.Set("Filters", "IsUnplayed")
	}
	if f.LibraryID != "" {
		q.Set("ParentId", f.LibraryID)
	}
	if f.MaxMovies > 0 {
		q.Set("Limit", strconv.Itoa(f.MaxMovies))
	}
}

// yearsList expands a [min,max] range into the comma-separated list of years
// Jellyfin's Years param expects (it does not accept a range). A zero bound
// falls back to the other bound (a single year).
func yearsList(min, max int) string {
	if min == 0 {
		min = max
	}
	if max == 0 {
		max = min
	}
	if min > max {
		min, max = max, min
	}
	years := make([]string, 0, max-min+1)
	for y := min; y <= max; y++ {
		years = append(years, strconv.Itoa(y))
	}
	return strings.Join(years, ",")
}
