package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// reloadTestServer builds a server with a stub source configured, so a
// configuration change has something to rebuild.
func reloadTestServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	clearConfigEnv(t)
	cfg.SessionTTL = "1h"
	cfg.CacheDir = t.TempDir()
	return New(cfg)
}

// activeSession creates a session and puts it past the lobby, so ending it has
// something to report.
func activeSession(t *testing.T, s *Server) *Session {
	t.Helper()
	session := s.store.Create("host")
	session.mu.Lock()
	session.Status = StatusActive
	session.Deck = []Movie{{ID: "tmdb:1", Title: "A"}, {ID: "tmdb:2", Title: "B"}}
	session.Votes["tmdb:1"] = map[string]bool{session.HostID: true}
	session.mu.Unlock()
	return session
}

func TestApplyConfigNoChangeIsANoOp(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t", PublicURL: "http://nas:8080"}
	s := reloadTestServer(t, cfg)
	before := s.currentSources()
	session := activeSession(t, s)

	live := s.config()
	if got := s.applyConfig(live); got != reloadNoChange {
		t.Errorf("outcome = %q, want %q", got, reloadNoChange)
	}
	if s.currentSources() != before {
		t.Error("the source set was rebuilt for a save that changed nothing")
	}
	session.mu.Lock()
	status := session.Status
	session.mu.Unlock()
	if status != StatusActive {
		t.Errorf("session status = %q, want it untouched by a no-op save", status)
	}
}

func TestApplyConfigHarmlessChangeKeepsSessions(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t", PublicURL: "http://nas:8080"}
	s := reloadTestServer(t, cfg)
	before := s.currentSources()
	session := activeSession(t, s)

	next := s.config()
	next.PublicURL = "http://corrected:8080"
	if got := s.applyConfig(next); got != reloadApplied {
		t.Errorf("outcome = %q, want %q", got, reloadApplied)
	}
	// Correcting a typo in the public URL must not end a movie night.
	session.mu.Lock()
	status := session.Status
	session.mu.Unlock()
	if status != StatusActive {
		t.Errorf("session status = %q, want a harmless change to leave it running", status)
	}
	if s.currentSources() != before {
		t.Error("a harmless change rebuilt the source set")
	}
	if s.config().PublicURL != "http://corrected:8080" {
		t.Error("the harmless value was not applied")
	}
}

func TestApplyConfigSourceChangeSwapsSetAndEndsSessions(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t", PublicURL: "http://nas:8080"}
	s := reloadTestServer(t, cfg)
	before := s.currentSources()
	session := activeSession(t, s)

	next := s.config()
	next.PlexURL = "http://moved:32400"
	if got := s.applyConfig(next); got != reloadApplied {
		t.Errorf("outcome = %q, want %q", got, reloadApplied)
	}
	if s.currentSources() == before {
		t.Error("the source set was not replaced after a source-affecting change")
	}
	session.mu.Lock()
	status := session.Status
	session.mu.Unlock()
	if status != StatusEnded {
		t.Errorf("session status = %q, want %q", status, StatusEnded)
	}
}

func TestApplyConfigPortChangeRequiresRestart(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t", Port: "8080"}
	s := reloadTestServer(t, cfg)
	session := activeSession(t, s)

	next := s.config()
	next.Port = "9090"
	if got := s.applyConfig(next); got != reloadRestartRequired {
		t.Errorf("outcome = %q, want %q", got, reloadRestartRequired)
	}
	// The listener cannot be rebound, but the server must keep serving on the
	// port it has rather than ending anything.
	session.mu.Lock()
	status := session.Status
	session.mu.Unlock()
	if status != StatusActive {
		t.Errorf("session status = %q, want a port change to leave sessions running", status)
	}
}

func TestEndAllBroadcastsLeaderboardAndReason(t *testing.T) {
	s := reloadTestServer(t, Config{PlexURL: "http://plex.local:32400", PlexToken: "t"})
	session := activeSession(t, s)

	client := &Client{
		send:          make(chan []byte, 8),
		done:          make(chan struct{}),
		session:       session,
		participantID: session.HostID,
	}
	session.mu.Lock()
	session.clients[session.HostID] = client
	session.mu.Unlock()

	if ended := s.store.EndAll(EndReasonReconfigured); ended != 1 {
		t.Fatalf("ended = %d, want 1", ended)
	}

	select {
	case raw := <-client.send:
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if env.Type != "session_ended" {
			t.Fatalf("type = %q, want session_ended", env.Type)
		}
		var payload SessionEndedPayload
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Reason != EndReasonReconfigured {
			t.Errorf("reason = %q, want %q", payload.Reason, EndReasonReconfigured)
		}
		if len(payload.Leaderboard) != 2 {
			t.Errorf("leaderboard has %d entries, want the whole deck", len(payload.Leaderboard))
		}
	default:
		t.Fatal("no session_ended message was broadcast")
	}
}

// TestEndAllOnLobbySession guards the empty case: a session that never started
// has no deck and no votes, and must not panic on the way out.
func TestEndAllOnLobbySession(t *testing.T) {
	s := reloadTestServer(t, Config{PlexURL: "http://plex.local:32400", PlexToken: "t"})
	s.store.Create("host")

	if ended := s.store.EndAll(EndReasonReconfigured); ended != 1 {
		t.Errorf("ended = %d, want 1", ended)
	}
}

func TestEndAllSkipsAlreadyFinishedSessions(t *testing.T) {
	s := reloadTestServer(t, Config{PlexURL: "http://plex.local:32400", PlexToken: "t"})
	session := s.store.Create("host")
	session.mu.Lock()
	session.Status = StatusMatched
	session.mu.Unlock()

	if ended := s.store.EndAll(EndReasonReconfigured); ended != 0 {
		t.Errorf("ended = %d, want 0: a matched session is already finished", ended)
	}
}

// TestConcurrentReloadAndReads is the reason the source set is an atomic
// pointer. Under -race this fails if a reader can observe a partially replaced
// set or race the configuration swap.
func TestConcurrentReloadAndReads(t *testing.T) {
	cfg := Config{PlexURL: "http://plex.local:32400", PlexToken: "t", PublicURL: "http://nas:8080"}
	s := reloadTestServer(t, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				set := s.currentSources()
				_ = configuredSources(set.sources, set.order)
				_ = s.config().PublicURL

				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/setup", nil))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			next := s.config()
			if j%2 == 0 {
				next.PlexURL = "http://a:32400"
			} else {
				next.PlexURL = "http://b:32400"
			}
			s.applyConfig(next)
		}
	}()
	wg.Wait()
}

func TestSettingsSaveReportsOutcome(t *testing.T) {
	s, _, token := newSettingsServer(t, "plex:\n  url: http://plex.local:32400\n  token: t\n")

	// A save that changes nothing must say so rather than claiming to have
	// applied something.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, settingsRequest{}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got settingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Outcome != string(reloadNoChange) {
		t.Errorf("outcome = %q, want %q", got.Outcome, reloadNoChange)
	}

	// A source-affecting save applies live.
	moved := "http://moved:32400"
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, settingsRequestFor(t, http.MethodPost, token, settingsRequest{
		Plex: &plexRequest{URL: &moved},
	}))
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Outcome != string(reloadApplied) {
		t.Errorf("outcome = %q, want %q (body %s)", got.Outcome, reloadApplied, rec.Body.String())
	}
	if got.RestartRequired {
		t.Error("restartRequired must be false for a change applied live")
	}
}

// TestApplyConfigSessionTTLReachesTheStore guards a setting that lives outside
// Config: storing a new TTL without pushing it to the store would report the
// change as applied while the sweeper kept expiring sessions on the old value.
func TestApplyConfigSessionTTLReachesTheStore(t *testing.T) {
	s := reloadTestServer(t, Config{PlexURL: "http://plex.local:32400", PlexToken: "t"})
	session := activeSession(t, s)

	next := s.config()
	next.SessionTTL = "30m"
	if got := s.applyConfig(next); got != reloadApplied {
		t.Fatalf("outcome = %q, want %q", got, reloadApplied)
	}

	s.store.mu.Lock()
	ttl := s.store.ttl
	s.store.mu.Unlock()
	if ttl != 30*time.Minute {
		t.Errorf("store ttl = %v, want 30m: a TTL change must reach the sweeper", ttl)
	}

	// A TTL change is harmless and must not end anything.
	session.mu.Lock()
	status := session.Status
	session.mu.Unlock()
	if status != StatusActive {
		t.Errorf("session status = %q, want a TTL change to leave sessions running", status)
	}
}
