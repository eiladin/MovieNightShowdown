package server

import (
	"context"
	"log"
	"time"
)

// sourceSet is the set of movie sources a deployment currently offers, held as
// one value so it can be replaced atomically.
//
// It is immutable once built. A configuration change produces a new set and
// swaps the pointer; nothing ever mutates the maps in place. That is what lets
// readers work without a lock: a request loads the pointer once and holds a
// consistent view for its whole duration, even while a reload is happening.
type sourceSet struct {
	sources  map[SourceID]MovieSource
	fetchers map[SourceID]PosterFetcher
	// order is this deployment's canonical source order: local libraries first
	// in alphabetical order, then the streaming services as configured.
	order []SourceID
}

// currentSources returns the live source set.
//
// Call it once per request and use the returned value throughout. Loading it
// twice within one request can straddle a reload and yield two different
// configurations, which is how a poster proxy ends up looking for a fetcher
// that the search it is serving never used.
func (s *Server) currentSources() *sourceSet {
	return s.sources.Load()
}

// buildSourceSet constructs the sources a configuration calls for.
//
// Startup and reload share this one path deliberately. Two construction paths
// drift, and the failure mode is a deployment whose behaviour depends on
// whether a setting was present at boot or saved later.
func buildSourceSet(cfg Config) *sourceSet {
	set := &sourceSet{
		sources:  map[SourceID]MovieSource{},
		fetchers: map[SourceID]PosterFetcher{},
	}

	// Each source is gated on its own credentials. Registering one
	// unconditionally would advertise a source every query fails against.
	if cfg.JellyfinConfigured() {
		jellyfin := NewJellyfinClient(cfg)
		set.sources[SourceJellyfin] = jellyfin
		set.fetchers[SourceJellyfin] = jellyfin
		set.order = append(set.order, SourceJellyfin)
	}
	// Plex sits after Jellyfin: the canonical order decides whose genre names
	// win in the merged vocabulary (see gatherVocabulary), and Jellyfin's stay
	// canonical so adding Plex cannot relabel an existing deployment's picker.
	if cfg.PlexConfigured() {
		plex := NewPlexClient(cfg)
		set.sources[SourcePlex] = plex
		set.fetchers[SourcePlex] = plex
		set.order = append(set.order, SourcePlex)
	}
	// Resolution needs the network for anything outside the built-in table, so
	// it is bounded and non-fatal: whatever resolves is offered, and the rest
	// is logged. It is skipped entirely without a read token, so an unset TMDB
	// token never advertises a streaming source at all.
	ctx, cancel := context.WithTimeout(context.Background(), providerResolveTimeout)
	defer cancel()
	for _, p := range resolveStreamingProviders(ctx, cfg, cfg.StreamingProviders) {
		src := NewTMDBSource(cfg, p)
		if src == nil {
			continue
		}
		if _, clash := set.sources[p.ID]; clash {
			log.Printf("server: skipping duplicate source %q", p.ID)
			continue
		}
		set.sources[p.ID] = src
		set.fetchers[p.ID] = src
		set.order = append(set.order, p.ID)
	}
	if len(set.sources) == 0 {
		log.Print("server: no movie source is configured — set JELLYFIN_URL and " +
			"JELLYFIN_API_KEY or PLEX_URL and PLEX_TOKEN for a local library, " +
			"and/or TMDB_READ_TOKEN for streaming services. Open the app for " +
			"setup instructions.")
	}
	return set
}

// reloadOutcome is what a saved configuration did to the running server.
type reloadOutcome string

const (
	// reloadNoChange means nothing that affects behaviour differed.
	reloadNoChange reloadOutcome = "no_change"
	// reloadApplied means the change took effect without a restart.
	reloadApplied reloadOutcome = "applied"
	// reloadRestartRequired means the change is persisted but cannot take
	// effect until the process restarts.
	reloadRestartRequired reloadOutcome = "restart_required"
)

// sourcesDiffer reports whether two configurations would produce different
// sources.
//
// Rebuilding ends every active session, so this must be true only when
// something genuinely source-affecting changed. It compares resolved values
// rather than file contents: a reordered or reformatted config file carrying
// identical values is not a change, and ending four people's movie night over
// a whitespace edit would be indefensible.
func sourcesDiffer(a, b Config) bool {
	if a.JellyfinURL != b.JellyfinURL ||
		a.JellyfinAPIKey != b.JellyfinAPIKey ||
		a.JellyfinUserID != b.JellyfinUserID {
		return true
	}
	if a.PlexURL != b.PlexURL ||
		a.PlexToken != b.PlexToken ||
		a.PlexLibrarySection != b.PlexLibrarySection {
		return true
	}
	if a.TMDBReadToken != b.TMDBReadToken ||
		a.TMDBWatchRegion != b.TMDBWatchRegion {
		return true
	}
	if len(a.StreamingProviders) != len(b.StreamingProviders) {
		return true
	}
	for i := range a.StreamingProviders {
		if a.StreamingProviders[i] != b.StreamingProviders[i] {
			return true
		}
	}
	return false
}

// harmlessDiffer reports whether settings changed that can be applied without
// touching sources or sessions. Correcting a typo in the public URL must not
// end a movie night.
func harmlessDiffer(a, b Config) bool {
	return a.PublicURL != b.PublicURL || a.SessionTTL != b.SessionTTL
}

// restartRequiredDiffer reports whether settings changed that cannot take
// effect in a running process. The listener is already bound to a port and
// cannot be rebound under live connections.
//
// Only the port qualifies. The cache directory is environment-only and can
// never differ between two resolved configurations of one process.
func restartRequiredDiffer(a, b Config) bool {
	return a.Port != b.Port
}

// applyConfig makes a saved configuration live, returning what it did.
//
// The three tiers are handled separately because rebuilding sources ends every
// active session. A save that changed nothing source-affecting must not cost
// anyone their movie night, and a save that changed only the listen port cannot
// take effect at all until the process restarts.
func (s *Server) applyConfig(next Config) reloadOutcome {
	s.cfgMu.RLock()
	current := s.cfg
	s.cfgMu.RUnlock()

	sourcesChanged := sourcesDiffer(current, next)
	harmless := harmlessDiffer(current, next)
	needsRestart := restartRequiredDiffer(current, next)

	if !sourcesChanged && !harmless && !needsRestart {
		log.Print("config: saved, but nothing that affects behaviour changed")
		return reloadNoChange
	}

	if !sourcesChanged {
		// Harmless settings apply without touching sources, so no session is
		// disturbed. The port is stored for the next start either way.
		s.setConfig(next)
		if needsRestart {
			log.Print("config: applied; a restart is needed for the listen port or cache directory")
			return reloadRestartRequired
		}
		log.Print("config: applied without rebuilding sources")
		return reloadApplied
	}

	// Build before ending anything: a failure here must not leave the server
	// with no sessions and no sources.
	set := buildSourceSet(next)

	// End sessions before the swap. Swapping first would leave in-flight
	// requests holding a deck dealt from sources that no longer exist, and the
	// posters for those cards would start returning 404 mid-swipe.
	ended := s.store.EndAll(EndReasonReconfigured)

	s.setConfig(next)
	s.sources.Store(set)
	log.Printf("config: sources rebuilt; %d session(s) ended", ended)

	if needsRestart {
		return reloadRestartRequired
	}
	return reloadApplied
}

// setConfig replaces the live configuration and propagates the settings that
// live outside it. A value stored here but not pushed to its owner would be
// reported as applied while nothing had changed.
func (s *Server) setConfig(next Config) {
	s.cfgMu.Lock()
	s.cfg = next
	s.cfgMu.Unlock()

	if ttl, err := time.ParseDuration(next.SessionTTL); err == nil {
		s.store.SetTTL(ttl)
	} else {
		log.Printf("config: invalid session TTL %q, keeping the previous value: %v", next.SessionTTL, err)
	}
}

// config returns the live configuration.
func (s *Server) config() Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}
