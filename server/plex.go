package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PlexClient talks to a Plex Media Server's HTTP API.
//
// Plex is a peer of Jellyfin, not a variant of it: the two share the Movie
// type and the MovieSource interface and nothing else. Their query params,
// their JSON shapes, and their rating scales all differ, so the mapping lives
// here rather than in a shared abstraction.
type PlexClient struct {
	baseURL string
	token   string
	// section is the library section key holding movies. When empty it is
	// discovered on first use, since a server may have several sections and
	// only one of them is type "movie".
	section string
	http    *http.Client

	// discoverOnce guards lazy section discovery. Discovery needs the network,
	// so it cannot happen in the constructor without making startup depend on
	// Plex being reachable.
	discoverOnce sync.Once
	discoverErr  error
}

// NewPlexClient builds a client from the server Config.
func NewPlexClient(cfg Config) *PlexClient {
	return &PlexClient{
		baseURL: strings.TrimRight(cfg.PlexURL, "/"),
		token:   cfg.PlexToken,
		section: cfg.PlexLibrarySection,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// ID identifies this source. PlexClient implements MovieSource.
func (c *PlexClient) ID() SourceID { return SourcePlex }

// Name implements NamedSource.
func (c *PlexClient) Name() string { return "Plex" }

// FetchDepth implements DepthedSource. Like Jellyfin, a local library is cheap
// to page through, so it contributes more candidates than a remote catalog.
func (c *PlexClient) FetchDepth() int { return jellyfinFetchDepth }

// SupportsUnwatched implements UnwatchedSource. Unlike Jellyfin, no extra
// configuration is needed: a Plex token identifies a user, so play state is
// always a question the credential can answer.
func (c *PlexClient) SupportsUnwatched() bool { return true }

// plexResponse is the envelope every Plex endpoint returns.
type plexResponse struct {
	MediaContainer struct {
		TotalSize int          `json:"totalSize"`
		Size      int          `json:"size"`
		Metadata  []plexItem   `json:"Metadata"`
		Directory []plexDirect `json:"Directory"`
	} `json:"MediaContainer"`
}

// plexDirect is one entry of a Directory listing: a library section, or a
// filter value such as a genre or content rating.
type plexDirect struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Title string `json:"title"`
	Tag   string `json:"tag"`
}

// name returns the display value of a Directory entry, which Plex calls Title
// on sections and Tag on filter values.
func (d plexDirect) name() string {
	if d.Tag != "" {
		return d.Tag
	}
	return d.Title
}

type plexItem struct {
	RatingKey string `json:"ratingKey"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Summary   string `json:"summary"`
	// ContentRating is already in the same vocabulary as Jellyfin's
	// OfficialRating ("PG-13"), so it needs no translation.
	ContentRating string `json:"contentRating"`
	// Rating is a critic score and AudienceRating an audience score. Only the
	// latter is comparable with Jellyfin's CommunityRating; see toMovie.
	Rating         float64 `json:"rating"`
	AudienceRating float64 `json:"audienceRating"`
	// Duration is milliseconds, where Jellyfin reports 100ns ticks.
	Duration int64  `json:"duration"`
	Thumb    string `json:"thumb"`
	// Genre is truncated by Plex to the first two tags in a list response,
	// even for a film its own detail endpoint reports five for. It is
	// therefore display data only: the genre filter is applied server-side by
	// Plex against the untruncated tags, and the picker's vocabulary comes
	// from the section's genre endpoint, so neither depends on this field
	// being complete. Filling it in would cost one detail request per movie —
	// 151 requests to deal a 150-card shoe — to decorate a poster card.
	Genre []plexDirect `json:"Genre"`
	Guid  []plexGUID   `json:"Guid"`
	// LegacyGUID is Plex's own "plex://movie/..." identifier. It exists here
	// only so it has an exact match to land in: encoding/json falls back to a
	// case-insensitive match, so without this field the string "guid" is
	// decoded into Guid ("[]plexGUID") and the whole response fails.
	LegacyGUID string `json:"guid"`
}

type plexGUID struct {
	ID string `json:"id"`
}

// get issues an authenticated GET against the Plex server and decodes the
// response.
//
// The status is checked before decoding because Plex's error path ignores the
// Accept header entirely and returns an HTML body; handing that to the decoder
// would report "invalid character '<'" instead of "unauthorized".
func (c *PlexClient) get(ctx context.Context, path string, q url.Values) (*plexResponse, error) {
	reqURL := c.baseURL + path
	if len(q) > 0 {
		reqURL += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex: GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex: GET %s returned %s", path, resp.Status)
	}

	var parsed plexResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("plex: decode %s response: %w", path, err)
	}
	return &parsed, nil
}

// movieSection returns the library section key to query, discovering it on
// first use when PLEX_LIBRARY_SECTION is unset.
//
// Discovery picks the first section of type "movie". A server with several
// movie sections therefore needs the setting; guessing would silently deal
// from whichever section Plex happened to list first.
func (c *PlexClient) movieSection(ctx context.Context) (string, error) {
	if c.section != "" {
		return c.section, nil
	}
	c.discoverOnce.Do(func() {
		parsed, err := c.get(ctx, "/library/sections", nil)
		if err != nil {
			c.discoverErr = err
			return
		}
		for _, d := range parsed.MediaContainer.Directory {
			if d.Type == "movie" {
				c.section = d.Key
				return
			}
		}
		c.discoverErr = fmt.Errorf("plex: no library section of type \"movie\"; set PLEX_LIBRARY_SECTION")
	})
	if c.discoverErr != nil {
		return "", c.discoverErr
	}
	if c.section == "" {
		return "", fmt.Errorf("plex: movie library section is unknown")
	}
	return c.section, nil
}

// Search implements MovieSource by delegating to Movies and discarding the
// total count, which only the library preview endpoint needs.
func (c *PlexClient) Search(ctx context.Context, f Filters) ([]Movie, error) {
	movies, _, err := c.Movies(ctx, f)
	return movies, err
}

// Movies fetches movies from Plex, applying filters, and maps them onto the
// Movie type used by the rest of the app.
//
// Like the Jellyfin client it returns both the (capped) list and the true
// total matching the filters, which Plex reports as totalSize independently of
// the container size.
func (c *PlexClient) Movies(ctx context.Context, filters Filters) ([]Movie, int, error) {
	section := filters.LibraryID
	if section == "" {
		var err error
		section, err = c.movieSection(ctx)
		if err != nil {
			return nil, 0, err
		}
	}

	q := url.Values{}
	q.Set("type", "1") // 1 = movie
	// Without this the Guid array is omitted, and with it the TMDB id that
	// lets a Plex item merge with the same film from a streaming source.
	q.Set("includeGuids", "1")
	filters.applyPlex(q)

	parsed, err := c.get(ctx, "/library/sections/"+url.PathEscape(section)+"/all", q)
	if err != nil {
		return nil, 0, err
	}

	movies := make([]Movie, 0, len(parsed.MediaContainer.Metadata))
	for _, it := range parsed.MediaContainer.Metadata {
		// Plex has no server-side minimum-rating filter, so it is applied here.
		// Doing it after the fetch means the rating floor narrows the sample
		// rather than the query, which is acceptable for a filter the host
		// uses to exclude a tail.
		m := it.toMovie()
		if filters.RatingMin > 0 && m.CommunityRating < filters.RatingMin {
			continue
		}
		movies = append(movies, m)
	}

	total := parsed.MediaContainer.TotalSize
	if total == 0 {
		total = len(movies)
	}
	return movies, total, nil
}

// applyPlex maps Filters onto a Plex /library/sections/{key}/all query.
//
// It is deliberately separate from apply (the Jellyfin form) rather than
// generalized: the two servers agree on no param name, no delimiter, and no
// range syntax, so a shared applier would be a switch statement wearing an
// interface. RatingMin is absent because Plex has no equivalent param; Movies
// applies it after the fetch.
func (f Filters) applyPlex(q url.Values) {
	if len(f.Genres) > 0 {
		// Plex ORs repeated values for one field with a "," delimiter.
		q.Set("genre", strings.Join(f.Genres, ","))
	}
	if f.YearMin > 0 || f.YearMax > 0 {
		// Plex's range operators are inconsistent across server versions, but
		// an explicit value list is honored the same way Jellyfin's Years is.
		q.Set("year", yearsList(f.YearMin, f.YearMax))
	}
	if len(f.OfficialRatings) > 0 {
		q.Set("contentRating", strings.Join(f.OfficialRatings, ","))
	}
	if f.Unwatched {
		q.Set("unwatched", "1")
	}
	if f.Limit > 0 {
		// Randomize server-side for the same reason Jellyfin does: without it
		// the cap takes the first N of title order and every session is dealt
		// the same deck.
		q.Set("sort", "random")
		q.Set("X-Plex-Container-Start", "0")
		q.Set("X-Plex-Container-Size", strconv.Itoa(f.Limit))
	}
}

// toMovie maps one Plex item onto the shared Movie type.
func (it plexItem) toMovie() Movie {
	posterURL := "/api/images/" + string(SourcePlex) + "/" + it.RatingKey
	if tag := plexThumbTag(it.Thumb); tag != "" {
		posterURL += "?tag=" + url.QueryEscape(tag)
	}
	// Prefer the TMDB id so a library item and the same film from a streaming
	// source collapse into one deck entry, exactly as the Jellyfin mapper does.
	// Items without one fall back to a Plex-namespaced id and never merge.
	id := "plex:" + it.RatingKey
	if tmdb := plexTMDBID(it.Guid); tmdb != "" {
		id = "tmdb:" + tmdb
	}
	genres := make([]string, 0, len(it.Genre))
	for _, g := range it.Genre {
		genres = append(genres, g.name())
	}
	// AudienceRating is the community score and shares Jellyfin's 0-10 scale
	// and population. Rating is a critic score, so it is only a fallback: a
	// deck mixing sources must not compare critics against audiences in the
	// leaderboard tiebreak.
	community := it.AudienceRating
	if community == 0 {
		community = it.Rating
	}
	return Movie{
		ID:              id,
		Title:           it.Title,
		Year:            it.Year,
		Genres:          genres,
		Overview:        it.Summary,
		Runtime:         int(it.Duration / 1000 / 60),
		CommunityRating: community,
		OfficialRating:  it.ContentRating,
		PosterURL:       posterURL,
		Availability:    []Availability{{Source: SourcePlex, Label: "Plex"}},
	}
}

// plexTMDBID returns the numeric TMDB id from a Plex Guid list, or "" when the
// item has none (an unmatched item, or one matched by a non-TMDB agent).
func plexTMDBID(guids []plexGUID) string {
	for _, g := range guids {
		if rest, ok := strings.CutPrefix(g.ID, "tmdb://"); ok {
			return rest
		}
	}
	return ""
}

// plexThumbTag extracts the version stamp from a thumb path such as
// "/library/metadata/8014/thumb/1782791305". It is the structural twin of
// Jellyfin's Primary image tag: Plex changes it when the artwork changes, so
// using it as the cache key pins the exact bytes.
func plexThumbTag(thumb string) string {
	i := strings.LastIndex(thumb, "/")
	if i < 0 || i == len(thumb)-1 {
		return ""
	}
	return thumb[i+1:]
}

// Vocabulary implements VocabularySource, reporting the genres and content
// ratings actually present in the movie section so the picker offers exactly
// what is on the shelf.
func (c *PlexClient) Vocabulary(ctx context.Context) (AvailableFilters, error) {
	section, err := c.movieSection(ctx)
	if err != nil {
		return AvailableFilters{}, err
	}
	genres, err := c.filterValues(ctx, section, "genre")
	if err != nil {
		return AvailableFilters{}, err
	}
	ratings, err := c.filterValues(ctx, section, "contentRating")
	if err != nil {
		return AvailableFilters{}, err
	}
	return AvailableFilters{Genres: genres, OfficialRatings: ratings}, nil
}

// filterValues lists one filter field's values for a section.
func (c *PlexClient) filterValues(ctx context.Context, section, field string) ([]string, error) {
	parsed, err := c.get(ctx, "/library/sections/"+url.PathEscape(section)+"/"+field, nil)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(parsed.MediaContainer.Directory))
	for _, d := range parsed.MediaContainer.Directory {
		if name := d.name(); name != "" {
			out = append(out, name)
		}
	}
	return out, nil
}

// fetchPoster downloads a movie's thumbnail from Plex. The tag is the thumb
// path's version stamp, which pins the exact image version so the cache key
// and the bytes agree.
func (c *PlexClient) fetchPoster(ctx context.Context, id, tag string) ([]byte, error) {
	path := "/library/metadata/" + url.PathEscape(id) + "/thumb"
	if tag != "" {
		path += "/" + url.PathEscape(tag)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Plex-Token", c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plex: fetch poster %s: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plex: poster %s returned %s", id, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
