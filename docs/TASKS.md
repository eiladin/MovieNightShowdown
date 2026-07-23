# Movie Night Showdown — Junior Developer Build Checklist

This is a step-by-step build sheet. Do the steps **in order, top to bottom**.
Each `- [ ]` is one task; check it off (`- [x]`) only after it works. Do not skip
ahead. Do not substitute libraries or versions — the choices below are final and
the rest of the plan depends on them.

If you were told "continue": read `docs/STATE.md` first to find which step you are
on, then come here.

---

## How to work (read this once, then follow it every step)

- **One step at a time.** Finish and verify a step before starting the next.
- **Copy the code blocks exactly.** Where a file's full contents are given,
      create the file with exactly that content. Where a command is given, run it
      exactly.
- **Verify after every step.** Each step ends with a "Verify:" line. If it
      does not pass, fix it before moving on. Do not check the box until Verify
      passes.
- **Tick the box in THIS file the moment Verify passes.** Edit the task's
      `- [ ]` to `- [x]` in `docs/TASKS.md` immediately — not only in
      `docs/STATE.md`. `docs/TASKS.md` is the running record of what is actually
      done; a completed task with an unchecked box is a bug. Before you hand back,
      confirm every task you finished this session is checked here.
- **Commit after each numbered group** (1.x, 2.x, …) with a Conventional
      Commit message, e.g. `feat(server): add health endpoint and SPA embed`.
- **Update `docs/STATE.md` before you stop** (any reason): set the current
      phase/status, set the single `Next action` to the next unchecked task here,
      and append a handback entry. Follow `docs/HANDOFF.md`.
- **Never delete or rewrite** past entries in the `docs/STATE.md` handoff log.
- **If a step fails and you cannot fix it in 3 tries:** stop, write exactly
      what failed (command + full error) into the `docs/STATE.md` handback log,
      set `Build: red (<reason>)`, and hand back. Do not guess-thrash.
- **Do not add libraries** that are not listed below. If you think you need
      one, stop and hand back a note instead.

---

## Locked tech choices (do NOT substitute)

**Backend**
- Go **1.23**. Module path: `github.com/eiladin/movie-night-showdown`.
- Router: **standard library** `net/http.ServeMux` with method+path patterns
  (Go 1.22+ style, e.g. `mux.HandleFunc("GET /healthz", ...)`). No chi, no gin.
- WebSocket: **`github.com/gorilla/websocket`** (latest v1.5.x). Nothing else.
- IDs: **`github.com/google/uuid`** for participant/session internal IDs.
- Session join codes: 4-char uppercase, custom helper (given in Phase 3).
- Storage: **in-memory only** (a `map` + `sync.Mutex`). No database in v1.

**Frontend** (in `web/`, package manager = **npm**)
- **Vite + React 18 + TypeScript**.
- Routing: **`react-router-dom`** v7 (installed in Phase 1; the `useParams` /
  `useSearchParams` APIs used here are the same as v6).
- State: **`zustand`**.
- Swipe cards: **`react-tinder-card`** (has built-in swipe + programmatic
  `swipe()` / `restoreCard()` for undo). No custom gesture engine.
- Confetti: **`canvas-confetti`**.
- QR code: **`react-qr-code`** (renders an SVG component).
- HTTP: native `fetch`. WebSocket: native `WebSocket`. No axios, no socket.io.
- Styling: **plain CSS** files (mobile-first). No Tailwind, no CSS-in-JS.

---

## Locked product rules (decided by the owner — do NOT change or interpret)

- **Quorum:** When the admin clicks "Begin", the current lobby roster is **locked**
  and `RequiredCount` = the number of locked-in participants (the admin may lower
  it before starting, never raise above the roster). A movie wins when **all
  locked participants** voted "yes" on it. New participants may join during Lobby
  but are **rejected once the session is Active**.
- **Deck cap:** The admin sets a "max movies" value in the filter panel,
  **default 50**. The deck is shuffled, then truncated to that many movies.
- **Admin access:** **Open on the LAN** — no password, no auth. Anyone who can
  reach the app can start a session and be admin.
- **Module path:** `github.com/eiladin/movie-night-showdown` (already set).

---

## Target repo layout (what it looks like when done)

```
movie-night-showdown/
  main.go                 # entrypoint: server, routes, embed
  server/
    server.go             # Server struct, router wiring
    health.go             # GET /healthz
    static.go             # SPA embed + fallback handler
    jellyfin.go           # Jellyfin REST client (Phase 2)
    images.go             # GET /api/images/{id} proxy (Phase 2)
    session.go            # Session/Participant types + store (Phase 3)
    hub.go                # WebSocket hub + client pumps (Phase 3)
    messages.go           # WS message envelope + payload types (Phase 3)
    match.go              # vote recording + match detection (Phase 4)
  go.mod / go.sum
  web/
    index.html
    package.json
    vite.config.ts
    src/
      main.tsx, App.tsx, store.ts, ws.ts, api.ts
      pages/  Landing.tsx  AdminSetup.tsx  Lobby.tsx  Swipe.tsx  Result.tsx
      components/  Card.tsx  Confetti.tsx  QRJoin.tsx
      styles/  *.css
    dist/                 # build output (gitignored; built before `go build`)
  Dockerfile
  docker-compose.yml
  .env.example
```

---

## Phases 1–6 — COMPLETE ✅

Phases 1–6 (scaffold, Jellyfin client + image proxy, sessions + WS hub, swipe
+ vote engine, reveal + leaderboard, polish + deploy) are fully built and
shipped. The granular build steps have been archived — see the git history and
the `docs/STATE.md` handoff log for what each phase delivered.

---

## Phase 7 — Poster caching & proactive warming

Goal: serve posters from an on-disk cache keyed by Jellyfin's image tag;
cache-bust on artwork changes; warm the cache at "go to the Lobby" so swiping
starts hot.

### 7.1 Add the `golang.org/x/sync` dependency
- [ ] From repo root run: `go get golang.org/x/sync@latest`
- [ ] Run `go mod tidy`
- Verify: `grep golang.org/x/sync go.mod` shows the require line; `go build ./...`
  passes.

### 7.2 Add `CACHE_DIR` to config — `server/config.go`
- [ ] Add the field to the `Config` struct (after `SessionTTL`):
  ```go
  	CacheDir string
  ```
- [ ] In `LoadConfig`, add to the `Config{...}` literal:
  ```go
  		CacheDir: os.Getenv("CACHE_DIR"),
  ```
- [ ] After the `SessionTTL` default block, add:
  ```go
  	if cfg.CacheDir == "" {
  		cfg.CacheDir = filepath.Join(os.TempDir(), "mns-posters")
  	}
  ```
- [ ] Add `"path/filepath"` to the imports.
- [ ] In `String()`, add `CacheDir` to the format string and args, e.g. append
  ` CacheDir=%s` and `c.CacheDir`.
- Verify: `go build ./...` passes.

### 7.3 Jellyfin: emit the image tag + add a poster fetch helper — `server/jellyfin.go`
- [ ] Add `"io"` to the imports.
- [ ] Add a field to `jellyfinItem` (after `OfficialRating`):
  ```go
  	ImageTags map[string]string `json:"ImageTags"`
  ```
- [ ] In `Movies`, replace the `PosterURL: "/api/images/" + it.ID,` line inside
  the mapping loop with a tag-aware URL. Just before building `m`, add:
  ```go
  		posterURL := "/api/images/" + it.ID
  		if tag := it.ImageTags["Primary"]; tag != "" {
  			posterURL += "?tag=" + url.QueryEscape(tag)
  		}
  ```
  and set `PosterURL: posterURL,` in the `Movie{...}` literal.
- [ ] Add a fetch helper at the end of the file:
  ```go
  // fetchPoster downloads a movie's Primary poster from Jellyfin. A non-empty
  // tag pins the exact image version so the cache key and the bytes agree.
  func (c *JellyfinClient) fetchPoster(ctx context.Context, id, tag string) ([]byte, error) {
  	reqURL := fmt.Sprintf("%s/Items/%s/Images/Primary?maxWidth=600", c.baseURL, url.PathEscape(id))
  	if tag != "" {
  		reqURL += "&tag=" + url.QueryEscape(tag)
  	}
  	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
  	if err != nil {
  		return nil, err
  	}
  	req.Header.Set("X-Emby-Token", c.apiKey)
  	resp, err := c.http.Do(req)
  	if err != nil {
  		return nil, fmt.Errorf("jellyfin: fetch poster %s: %w", id, err)
  	}
  	defer resp.Body.Close()
  	if resp.StatusCode != http.StatusOK {
  		return nil, fmt.Errorf("jellyfin: poster %s returned %s", id, resp.Status)
  	}
  	return io.ReadAll(resp.Body)
  }
  ```
- Note: `ImageTags` is normally returned by `GET /Items` by default. Confirm in
  7.10 that `posterURL` includes `?tag=`; if it does not, add `ImageTags` to the
  `Fields` value in `Movies` and re-check.
- Verify: `go build ./...` passes.

### 7.4 New on-disk poster cache — `server/imagecache.go` (new file)
- [ ] Create `server/imagecache.go` with exactly:
  ```go
  package server

  import (
  	"context"
  	"log"
  	"net/url"
  	"os"
  	"path/filepath"
  	"strings"
  	"sync"
  	"time"

  	"golang.org/x/sync/singleflight"
  )

  // posterCache is a read-through, on-disk cache of Jellyfin poster images.
  // Files are named "{id}_{tag}"; when a poster's artwork changes in Jellyfin
  // its Primary image tag changes, so the key changes and the stale file is
  // pruned. singleflight collapses concurrent misses for the same poster into a
  // single upstream fetch.
  type posterCache struct {
  	dir   string
  	group singleflight.Group
  }

  // newPosterCache returns a cache rooted at dir, creating the directory. If dir
  // is empty or cannot be created it returns a disabled cache; callers then fall
  // back to a live proxy transparently (ensure still fetches, it just skips the
  // write).
  func newPosterCache(dir string) *posterCache {
  	if dir == "" {
  		return &posterCache{}
  	}
  	if err := os.MkdirAll(dir, 0o755); err != nil {
  		log.Printf("poster cache: disabled, cannot create %s: %v", dir, err)
  		return &posterCache{}
  	}
  	log.Printf("poster cache: enabled at %s", dir)
  	return &posterCache{dir: dir}
  }

  func (c *posterCache) enabled() bool { return c != nil && c.dir != "" }

  // sanitize keeps only filename-safe characters so an id/tag can never escape
  // the cache directory. Jellyfin ids/tags are hex, so this is defensive.
  func sanitize(s string) string {
  	return strings.Map(func(r rune) rune {
  		switch {
  		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
  			return r
  		default:
  			return '_'
  		}
  	}, s)
  }

  func (c *posterCache) path(id, tag string) string {
  	return filepath.Join(c.dir, sanitize(id)+"_"+sanitize(tag))
  }

  // get returns cached bytes for id+tag, or ok=false on a miss.
  func (c *posterCache) get(id, tag string) ([]byte, bool) {
  	if !c.enabled() {
  		return nil, false
  	}
  	data, err := os.ReadFile(c.path(id, tag))
  	if err != nil {
  		return nil, false
  	}
  	return data, true
  }

  // ensure guarantees id+tag is cached, fetching from Jellyfin on a miss.
  // Concurrent callers for the same id+tag share one fetch. It returns the image
  // bytes; a cache-write failure is logged but not fatal.
  func (c *posterCache) ensure(ctx context.Context, jf *JellyfinClient, id, tag string) ([]byte, error) {
  	if data, ok := c.get(id, tag); ok {
  		return data, nil
  	}
  	v, err, _ := c.group.Do(id+"_"+tag, func() (interface{}, error) {
  		if data, ok := c.get(id, tag); ok {
  			return data, nil // populated while we waited
  		}
  		data, err := jf.fetchPoster(ctx, id, tag)
  		if err != nil {
  			return nil, err
  		}
  		if c.enabled() {
  			c.store(id, tag, data)
  		}
  		return data, nil
  	})
  	if err != nil {
  		return nil, err
  	}
  	return v.([]byte), nil
  }

  // store writes data atomically (temp file + rename) and prunes older files for
  // the same id.
  func (c *posterCache) store(id, tag string, data []byte) {
  	tmp, err := os.CreateTemp(c.dir, "tmp-*")
  	if err != nil {
  		log.Printf("poster cache: temp create failed: %v", err)
  		return
  	}
  	tmpName := tmp.Name()
  	if _, err := tmp.Write(data); err != nil {
  		tmp.Close()
  		os.Remove(tmpName)
  		log.Printf("poster cache: write failed: %v", err)
  		return
  	}
  	if err := tmp.Close(); err != nil {
  		os.Remove(tmpName)
  		log.Printf("poster cache: close failed: %v", err)
  		return
  	}
  	if err := os.Rename(tmpName, c.path(id, tag)); err != nil {
  		os.Remove(tmpName)
  		log.Printf("poster cache: rename failed: %v", err)
  		return
  	}
  	c.pruneOld(id, tag)
  }

  // pruneOld removes every cached file for id whose tag differs from keepTag.
  func (c *posterCache) pruneOld(id, keepTag string) {
  	matches, _ := filepath.Glob(filepath.Join(c.dir, sanitize(id)+"_*"))
  	keep := c.path(id, keepTag)
  	for _, m := range matches {
  		if m != keep {
  			os.Remove(m)
  		}
  	}
  }

  // warm pre-fetches every movie's poster into the cache, bounded to a few
  // concurrent Jellyfin requests. Intended to run in a background goroutine.
  func (c *posterCache) warm(movies []Movie, jf *JellyfinClient) {
  	const workers = 6
  	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
  	defer cancel()

  	sem := make(chan struct{}, workers)
  	var wg sync.WaitGroup
  	for _, m := range movies {
  		id, tag := parsePosterRef(m.PosterURL)
  		if id == "" {
  			continue
  		}
  		if _, ok := c.get(id, tag); ok {
  			continue // already warm
  		}
  		wg.Add(1)
  		sem <- struct{}{}
  		go func(id, tag string) {
  			defer wg.Done()
  			defer func() { <-sem }()
  			if _, err := c.ensure(ctx, jf, id, tag); err != nil {
  				log.Printf("poster cache warm: %s: %v", id, err)
  			}
  		}(id, tag)
  	}
  	wg.Wait()
  }

  // parsePosterRef extracts the item id and image tag from a proxied poster URL
  // of the form "/api/images/{id}?tag={tag}".
  func parsePosterRef(posterURL string) (id, tag string) {
  	u, err := url.Parse(posterURL)
  	if err != nil {
  		return "", ""
  	}
  	return strings.TrimPrefix(u.Path, "/api/images/"), u.Query().Get("tag")
  }
  ```
- Verify: `go build ./...` passes (the cache is defined but not yet wired).

### 7.5 Wire the cache into the server (one group — build goes green at the end)
Edit three files together; run the group's Verify only after all three.

- [ ] `server/server.go`: add a `cache *posterCache` field to the `Server`
  struct (after `store`), construct it in `New` (in the `&Server{...}` literal,
  after `store: NewStore(ttl),`):
  ```go
  		cache: newPosterCache(cfg.CacheDir),
  ```
  and register the warm route in `routes()` after the preview/filters routes:
  ```go
  	s.mux.HandleFunc("POST /api/library/warm", s.handleLibraryWarm)
  ```
- [ ] `server/images.go`: replace the whole file body of `handleImage` with a
  cache-backed version (keep the package + `net/http` import; drop `fmt`, `io`,
  `net/url`):
  ```go
  package server

  import "net/http"

  // handleImage serves a movie's primary poster from the on-disk cache, fetching
  // from Jellyfin on a miss. Images are keyed by item id + Primary image tag
  // (the ?tag= query param). With a tag the response is immutable for a year,
  // because changed artwork gets a new tag and therefore a new URL.
  func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
  	id := r.PathValue("id")
  	if id == "" {
  		http.NotFound(w, r)
  		return
  	}
  	tag := r.URL.Query().Get("tag")

  	data, err := s.cache.ensure(r.Context(), s.jellyfin, id, tag)
  	if err != nil {
  		http.Error(w, "image not found", http.StatusNotFound)
  		return
  	}

  	w.Header().Set("Content-Type", http.DetectContentType(data))
  	if tag != "" {
  		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
  	} else {
  		w.Header().Set("Cache-Control", "public, max-age=86400")
  	}
  	w.WriteHeader(http.StatusOK)
  	_, _ = w.Write(data)
  }
  ```
- [ ] `server/library.go`: add the warm handler at the end of the file:
  ```go
  // handleLibraryWarm pre-fetches every poster for the filtered library into the
  // on-disk cache so the session starts warm. It returns the candidate count
  // immediately and warms in the background.
  func (s *Server) handleLibraryWarm(w http.ResponseWriter, r *http.Request) {
  	filters := ParseFilters(r.URL.Query())

  	movies, count, err := s.jellyfin.Movies(r.Context(), filters)
  	if err != nil {
  		log.Printf("library warm: %v", err)
  		http.Error(w, "failed to query Jellyfin library", http.StatusBadGateway)
  		return
  	}

  	if s.cache.enabled() {
  		go s.cache.warm(movies, s.jellyfin)
  	}

  	w.Header().Set("Content-Type", "application/json")
  	_ = json.NewEncoder(w).Encode(map[string]int{"count": count})
  }
  ```
- Verify: `go build ./... && go vet ./...` pass.

### 7.6 Frontend: warm the cache at "go to the Lobby"
- [ ] `web/src/api.ts`: add after `getPreview`:
  ```ts
  // warmLibrary asks the server to pre-fetch every poster for the filtered
  // library into its cache before the session starts. Returns the candidate
  // count; warming happens in the background server-side.
  export async function warmLibrary(filters: PreviewFilters): Promise<number> {
    const params = buildPreviewParams(filters)
    const res = await fetch(`/api/library/warm?${params.toString()}`, { method: 'POST' })
    if (!res.ok) {
      throw new Error(`warm request failed: ${res.status} ${res.statusText}`)
    }
    const body = (await res.json()) as { count: number }
    return body.count
  }
  ```
- [ ] `web/src/pages/AdminSetup.tsx`: add `warmLibrary` to the existing import
  from `'../api'`, and update `handleGoToLobby` (currently lines 100–102) to:
  ```tsx
    function handleGoToLobby() {
      setFilters(currentFilters())
      // Warm the poster cache during the lobby-fill window (fire-and-forget;
      // must never block entering the lobby).
      warmLibrary(currentFilters()).catch((err) =>
        console.error('Failed to warm poster cache:', err),
      )
    }
  ```
- Verify: `cd web && npm run build` succeeds (`tsc -b` + `vite build`, no errors).

### 7.7 Unit test — `server/imagecache_test.go` (new file)
- [ ] Add a test covering: `store` → `get` round-trip; `pruneOld` removes the
  old-tag file when a new tag is stored for the same id; `parsePosterRef` parses
  `/api/images/{id}?tag={tag}`; and `sanitize` strips path separators. Use
  `t.TempDir()` for the cache dir. (No Jellyfin needed — test the cache
  primitives directly, mirroring the skip-if-unconfigured style of
  `server/jellyfin_test.go`.)
- Verify: `go test ./server/... -run TestPosterCache -v` passes.

### 7.8 Config plumbing, deploy volume, and docs
- [ ] `docker-compose.yml`: add a named volume mounted at the container's cache
  dir and set `CACHE_DIR` to it, so the disk cache survives restarts. Example:
  add `- poster-cache:/var/cache/mns` under the service `volumes:`,
  `CACHE_DIR=/var/cache/mns` under `environment:`, and a top-level
  `volumes:\n  poster-cache:`.
- [ ] `README.md` and `CLAUDE.md`: add a `CACHE_DIR` row to the env-var table —
  "optional; directory for the on-disk poster cache (default a temp dir);
  mount a volume in Docker to persist it across restarts."
- Verify: `docker compose config` parses; the env table renders the new row.

### 7.9 (Stretch — optional UI) promote "go to the Lobby" to a distinct button
- [ ] In `web/src/pages/AdminSetup.tsx`, the "go to the Lobby" action is a
  `<Link>` (line 111). Restyle it as a primary button visually distinct from the
  secondary "Preview" button — reuse the existing scoped `.btn-primary` style
  (added in a recent commit). Keep `onClick={handleGoToLobby}` and the
  `to={`/join/${sessionCode}`}` navigation.
- Verify: `cd web && npm run build` succeeds; the two actions read as
  primary (commit) vs secondary (preview).

### 7.10 End-to-end verification + handback
- [ ] `go build ./... && go vet ./...` clean; `go mod tidy` leaves no diff;
  `go test ./...` passes; `cd web && npm run build` succeeds.
- [ ] Tag in URL: `curl -s "localhost:8080/api/library/preview?genres=Action" | jq '.movies[0].posterURL'`
  → shows `/api/images/{id}?tag=...`. (If not, apply the `Fields` fix noted in
  7.3.)
- [ ] Immutable header + cache write:
  `curl -sD - "localhost:8080/api/images/{id}?tag={tag}" -o /dev/null | grep -i cache-control`
  → `...immutable`; confirm a `{id}_{tag}` file appears in `CACHE_DIR`.
- [ ] Cache hit / dedup: fire several concurrent requests for one poster
  (`seq 8 | xargs -P8 -I_ curl -s -o /dev/null "localhost:8080/api/images/{id}?tag={tag}"`);
  confirm only one upstream fetch via Jellyfin access logs.
- [ ] Warming: `curl -s -X POST "localhost:8080/api/library/warm?genres=Action" | jq .count`
  returns a number immediately; watch `CACHE_DIR` fill shortly after. In the UI,
  clicking "go to the Lobby" triggers the warm and does not block navigation.
- [ ] Invalidation: change a poster in Jellyfin, re-run preview → new `tag` in
  the URL → new cache file written, old `{id}_{oldtag}` file pruned.
- [ ] Degradation: point `CACHE_DIR` at an unwritable path → images still serve
  (live proxy), server logs "poster cache: disabled", no crash.
- [ ] Handback: tick every finished box in `docs/TASKS.md`; update
  `docs/STATE.md` (status/Next action/checklist) and append a dated handback
  entry per `docs/HANDOFF.md`. Commit each group with Conventional Commits.
