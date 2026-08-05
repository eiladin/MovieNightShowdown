package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// providerListTTL bounds how long a region's provider list is reused.
//
// The list changes rarely, the settings screen re-requests it whenever the
// region changes, and TMDB rate-limits. A short TTL keeps a hand on the
// settings screen from generating one upstream call per keystroke without
// pinning a stale list for the life of the process.
const providerListTTL = 10 * time.Minute

// verifyTimeout bounds the single upstream call a verification makes. A
// settings screen waiting on a hung request is worse than one told the check
// timed out.
const verifyTimeout = 10 * time.Second

// providerCache memoizes TMDB's provider list per watch region.
//
// The key is the region alone: the list is a property of the region, not of
// whoever asked for it, so a candidate token and a stored token yield the same
// answer and may share an entry.
//
// That key is exactly why verification must not read this cache. Answering
// "is this token valid?" from an entry warmed by a different token answers a
// different question, and would report a garbage token as valid for as long as
// the entry lived. See fetchProviderList's allowCache parameter.
type providerCache struct {
	mu      sync.Mutex
	entries map[string]providerCacheEntry
}

type providerCacheEntry struct {
	providers []providerOption
	fetchedAt time.Time
}

// providerOption is one selectable streaming service.
type providerOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newProviderCache() *providerCache {
	return &providerCache{entries: map[string]providerCacheEntry{}}
}

func (c *providerCache) get(region string) ([]providerOption, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[region]
	if !ok || time.Since(entry.fetchedAt) > providerListTTL {
		return nil, false
	}
	return entry.providers, true
}

func (c *providerCache) put(region string, providers []providerOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[region] = providerCacheEntry{providers: providers, fetchedAt: time.Now()}
}

// verifyTMDBRequest carries a candidate token to check. The token is never
// persisted by this route and never echoed back.
type verifyTMDBRequest struct {
	Token  string `json:"token"`
	Region string `json:"region"`
}

type verifyTMDBResponse struct {
	Valid bool `json:"valid"`
	// Message explains a failure in terms an operator can act on. It never
	// contains the submitted token.
	Message string `json:"message,omitempty"`
}

// handleVerifyTMDB checks whether a candidate TMDB read token authenticates.
//
// Verifying before saving means a bad token is reported at the moment it is
// entered rather than at the start of a movie night, and it is what lets the
// provider picker appear only when it can actually be populated.
func (s *Server) handleVerifyTMDB(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}

	var req verifyTMDBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, verifyTMDBResponse{Message: "request body is not valid JSON"})
		return
	}
	// An empty token means "check the one already stored", the same fallback the
	// provider list uses. The settings screen never receives a stored credential,
	// so it has nothing to submit for one that is already saved — without this
	// fallback, checking a working stored token reported "no token was supplied"
	// and the screen hid the provider picker on the strength of it.
	token := req.Token
	if token == "" {
		token = s.config().TMDBReadToken
	}
	if token == "" {
		writeJSON(w, http.StatusOK, verifyTMDBResponse{Message: "no token was supplied"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), verifyTimeout)
	defer cancel()

	// allowCache is false: the cache is keyed by region alone, so serving a
	// verification from it would answer "is this token valid?" with "did
	// anyone ask about this region recently?" and report a garbage token as
	// valid. Verification must reach the upstream every time; that is the
	// entire point of the route.
	if _, err := s.fetchProviderList(ctx, token, s.regionOrDefault(req.Region), false); err != nil {
		// The upstream error is logged, not returned: it can name the request
		// URL, and the token travels in a header adjacent to it.
		writeJSON(w, http.StatusOK, verifyTMDBResponse{
			Message: "TMDB rejected the token, or could not be reached",
		})
		return
	}
	writeJSON(w, http.StatusOK, verifyTMDBResponse{Valid: true})
}

// providerListRequest asks for the providers selectable in a region. Token is
// optional: it lets the screen populate the picker from a token that has been
// verified but not yet saved.
type providerListRequest struct {
	Region string `json:"region"`
	Token  string `json:"token"`
}

type providerListResponse struct {
	Region    string           `json:"region"`
	Providers []providerOption `json:"providers"`
}

// handleProviderList returns the watch providers TMDB lists for a region, for
// the settings screen to render as a picker.
func (s *Server) handleProviderList(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeSetup(r) {
		unauthorized(w)
		return
	}

	var req providerListRequest
	// An empty body is legitimate: it means "the stored token, the stored
	// region".
	_ = json.NewDecoder(r.Body).Decode(&req)

	token := req.Token
	if token == "" {
		token = s.config().TMDBReadToken
	}
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"message": "a TMDB read token is required to list providers",
		})
		return
	}
	region := s.regionOrDefault(req.Region)

	ctx, cancel := context.WithTimeout(r.Context(), verifyTimeout)
	defer cancel()

	providers, err := s.fetchProviderList(ctx, token, region, true)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"message": "could not reach TMDB to list providers",
		})
		return
	}
	writeJSON(w, http.StatusOK, providerListResponse{Region: region, Providers: providers})
}

// regionOrDefault normalizes a requested region, falling back to the
// deployment's configured one.
func (s *Server) regionOrDefault(region string) string {
	if region != "" {
		return normalizeRegion(region)
	}
	if configured := s.config().TMDBWatchRegion; configured != "" {
		return configured
	}
	return defaultWatchRegion
}

// fetchProviderList returns a region's providers, from cache when allowCache is
// set and the entry is fresh.
//
// It goes through providerResolver rather than issuing its own request: adding
// a second path to the same TMDB endpoint would mean two places to fix when the
// upstream shape changes.
//
// allowCache exists because the cache key is the region alone. That is correct
// for populating a picker — the list does not depend on who asked — and wrong
// for deciding whether a credential authenticates, which is a question only the
// upstream can answer.
func (s *Server) fetchProviderList(ctx context.Context, token, region string, allowCache bool) ([]providerOption, error) {
	if allowCache {
		if cached, ok := s.providers.get(region); ok {
			return cached, nil
		}
	}

	cfg := s.config()
	resolver := newProviderResolver(Config{
		TMDBReadToken:   token,
		TMDBWatchRegion: region,
		tmdbBaseURL:     cfg.tmdbBaseURL,
	})
	list, err := resolver.fetch(ctx)
	if err != nil {
		return nil, err
	}

	// The catalog is shared with resolveStreamingProviders, so every option
	// offered here is one resolution can actually produce, under the same id.
	// It is unique by id and already ordered by name — both matter to the picker,
	// which keys its rows by id and would otherwise reorder itself between
	// requests for no reason the operator can see.
	cat := newProviderCatalog(list)
	options := make([]providerOption, 0, len(cat.options))
	for _, p := range cat.options {
		options = append(options, providerOption{ID: string(p.ID), Name: p.Name})
	}

	s.providers.put(region, options)
	return options, nil
}
