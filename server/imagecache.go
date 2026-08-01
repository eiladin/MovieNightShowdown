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

func (c *posterCache) path(source SourceID, id, tag string) string {
	return filepath.Join(c.dir, sanitize(string(source))+"_"+sanitize(id)+"_"+sanitize(tag))
}

// get returns cached bytes for source+id+tag, or ok=false on a miss.
func (c *posterCache) get(source SourceID, id, tag string) ([]byte, bool) {
	if !c.enabled() {
		return nil, false
	}
	data, err := os.ReadFile(c.path(source, id, tag))
	if err != nil {
		return nil, false
	}
	return data, true
}

// ensure guarantees source+id+tag is cached, fetching from the source's
// upstream on a miss. Concurrent callers for the same key share one fetch. It
// returns the image bytes; a cache-write failure is logged but not fatal.
func (c *posterCache) ensure(ctx context.Context, f PosterFetcher, source SourceID, id, tag string) ([]byte, error) {
	if data, ok := c.get(source, id, tag); ok {
		return data, nil
	}
	key := string(source) + "_" + id + "_" + tag
	v, err, _ := c.group.Do(key, func() (interface{}, error) {
		if data, ok := c.get(source, id, tag); ok {
			return data, nil // populated while we waited
		}
		// Detach from the caller's request context: this fetch is shared by all
		// concurrent callers for this poster, so one caller disconnecting must not
		// cancel the fetch for the others.
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		data, err := f.fetchPoster(fetchCtx, id, tag)
		if err != nil {
			return nil, err
		}
		if c.enabled() {
			c.store(source, id, tag, data)
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
func (c *posterCache) store(source SourceID, id, tag string, data []byte) {
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
	if err := os.Rename(tmpName, c.path(source, id, tag)); err != nil {
		os.Remove(tmpName)
		log.Printf("poster cache: rename failed: %v", err)
		return
	}
	c.pruneOld(source, id, tag)
}

// pruneOld removes every cached file for source+id whose tag differs from
// keepTag.
func (c *posterCache) pruneOld(source SourceID, id, keepTag string) {
	matches, _ := filepath.Glob(filepath.Join(c.dir, sanitize(string(source))+"_"+sanitize(id)+"_*"))
	keep := c.path(source, id, keepTag)
	for _, m := range matches {
		if m != keep {
			os.Remove(m)
		}
	}
}

// warm pre-fetches every movie's poster into the cache, bounded to a few
// concurrent upstream requests. Movies whose source has no registered fetcher
// are skipped. Intended to run in a background goroutine.
func (c *posterCache) warm(movies []Movie, fetchers map[SourceID]PosterFetcher) {
	const workers = 6
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, m := range movies {
		source, id, tag := parsePosterRef(m.PosterURL)
		if id == "" {
			continue
		}
		f, ok := fetchers[source]
		if !ok {
			continue
		}
		if _, ok := c.get(source, id, tag); ok {
			continue // already warm
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(f PosterFetcher, source SourceID, id, tag string) {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := c.ensure(ctx, f, source, id, tag); err != nil {
				log.Printf("poster cache warm: %s/%s: %v", source, id, err)
			}
		}(f, source, id, tag)
	}
	wg.Wait()
}

// parsePosterRef extracts the source, item id, and image tag from a proxied
// poster URL of the form "/api/images/{source}/{id}?tag={tag}". It returns an
// empty id if the URL does not have that shape.
func parsePosterRef(posterURL string) (source SourceID, id, tag string) {
	u, err := url.Parse(posterURL)
	if err != nil {
		return "", "", ""
	}
	rest := strings.TrimPrefix(u.Path, "/api/images/")
	if rest == u.Path {
		return "", "", "" // prefix was absent
	}
	src, itemID, found := strings.Cut(rest, "/")
	if !found || src == "" || itemID == "" {
		return "", "", ""
	}
	return SourceID(src), itemID, u.Query().Get("tag")
}
