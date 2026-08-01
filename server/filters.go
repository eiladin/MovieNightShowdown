package server

import (
	"net/url"
	"strconv"
	"strings"
)

const (
	// defaultDeckSize is the number of cards actually dealt into play when the
	// host does not choose a size. Applied after every source's results are
	// merged and shuffled.
	defaultDeckSize = 50

	// jellyfinFetchDepth and streamingFetchDepth are how many candidates each
	// source contributes to the shoe before the deck is cut from it. They are
	// deliberately larger than the deck: the shoe is the sample, the deck is
	// the hand. Jellyfin gets more because a home library is a single finite
	// catalog rather than one of several.
	jellyfinFetchDepth  = 150
	streamingFetchDepth = tmdbPagesPerProvider * tmdbPageSize // 60
)

// Filters holds the host's library filter selections, parsed from the query
// params of GET /api/library/preview, and also sent as JSON inside the
// host:start WS payload — hence the json tags, matching the
// frontend's PreviewFilters shape (web/src/api.ts).
type Filters struct {
	Genres          []string   `json:"genres"`
	YearMin         int        `json:"yearMin"`
	YearMax         int        `json:"yearMax"`
	RatingMin       float64    `json:"ratingMin"`       // minimum CommunityRating
	OfficialRatings []string   `json:"officialRatings"` // MPAA rating, e.g. ["PG", "PG-13"]
	Unwatched       bool       `json:"unwatched"`
	LibraryID       string     `json:"libraryId"`
	Sources         []SourceID `json:"sources"` // empty means Jellyfin only
	Limit           int        `json:"limit"`   // fetch cap for the preview/library query, default 50
}

// ParseFilters parses Filters from request query params.
func ParseFilters(q url.Values) Filters {
	f := Filters{
		Genres:          q["genres"],
		OfficialRatings: q["officialRatings"],
		Unwatched:       q.Get("unwatched") == "true",
		LibraryID:       q.Get("libraryId"),
		Limit:           jellyfinFetchDepth,
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
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		f.Limit = v
	}
	for _, s := range q["sources"] {
		f.Sources = append(f.Sources, SourceID(s))
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
	if len(f.OfficialRatings) > 0 {
		q.Set("OfficialRatings", strings.Join(f.OfficialRatings, "|"))
	}
	if f.Unwatched && hasUserID {
		q.Set("Filters", "IsUnplayed")
	}
	if f.LibraryID != "" {
		q.Set("ParentId", f.LibraryID)
	}
	if f.Limit > 0 {
		// Randomize server-side so the Limit takes a random sample of the
		// whole filtered library rather than the first N of Jellyfin's
		// default (SortName) order, which would deal the same deck to
		// every session.
		q.Set("SortBy", "Random")
		q.Set("Limit", strconv.Itoa(f.Limit))
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
