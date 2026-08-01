package server

import (
	"context"
	"errors"
	"testing"
)

type fakeSource struct {
	id     SourceID
	movies []Movie
	err    error
	gotLim int
	gotUnw bool
}

func (f *fakeSource) ID() SourceID { return f.id }
func (f *fakeSource) Search(ctx context.Context, flt Filters) ([]Movie, error) {
	f.gotLim = flt.Limit
	f.gotUnw = flt.Unwatched
	return f.movies, f.err
}

func TestSelectSourcesDefaultsToJellyfin(t *testing.T) {
	avail := map[SourceID]MovieSource{
		SourceJellyfin: &fakeSource{id: SourceJellyfin},
		SourceNetflix:  &fakeSource{id: SourceNetflix},
	}
	got := selectSources(avail, nil)
	if len(got) != 1 || got[0].ID() != SourceJellyfin {
		t.Fatalf("empty selection should fall back to Jellyfin alone, got %d sources", len(got))
	}
}

func TestSelectSourcesSkipsUnconfigured(t *testing.T) {
	avail := map[SourceID]MovieSource{SourceJellyfin: &fakeSource{id: SourceJellyfin}}
	got := selectSources(avail, []SourceID{SourceJellyfin, SourceDisney})
	if len(got) != 1 || got[0].ID() != SourceJellyfin {
		t.Fatalf("unconfigured sources must be skipped, got %d sources", len(got))
	}
}

func TestGatherShoeMergesAndSetsPerSourceDepth(t *testing.T) {
	jf := &fakeSource{id: SourceJellyfin, movies: []Movie{
		{ID: "tmdb:603", Availability: []Availability{{Source: SourceJellyfin}}},
	}}
	nf := &fakeSource{id: SourceNetflix, movies: []Movie{
		{ID: "tmdb:603", Availability: []Availability{{Source: SourceNetflix}}},
		{ID: "tmdb:78", Availability: []Availability{{Source: SourceNetflix}}},
	}}

	movies, failed, err := gatherShoe(context.Background(), []MovieSource{jf, nf}, Filters{Unwatched: true})
	if err != nil {
		t.Fatalf("gatherShoe: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failures, got %v", failed)
	}
	if len(movies) != 2 {
		t.Fatalf("expected 2 merged movies, got %d", len(movies))
	}
	if jf.gotLim != jellyfinFetchDepth {
		t.Fatalf("jellyfin fetch depth = %d, want %d", jf.gotLim, jellyfinFetchDepth)
	}
	if nf.gotLim != streamingFetchDepth {
		t.Fatalf("streaming fetch depth = %d, want %d", nf.gotLim, streamingFetchDepth)
	}
	if !jf.gotUnw {
		t.Fatalf("unwatched must be passed through to Jellyfin")
	}
	if nf.gotUnw {
		t.Fatalf("unwatched must never be passed to a streaming source")
	}
}

func TestGatherShoeDegradesOnPartialFailure(t *testing.T) {
	jf := &fakeSource{id: SourceJellyfin, movies: []Movie{{ID: "jf:a"}}}
	nf := &fakeSource{id: SourceNetflix, err: errors.New("upstream down")}

	movies, failed, err := gatherShoe(context.Background(), []MovieSource{jf, nf}, Filters{})
	if err != nil {
		t.Fatalf("partial failure must not be fatal, got %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("expected the surviving source's movies, got %d", len(movies))
	}
	if len(failed) != 1 || failed[0] != SourceNetflix {
		t.Fatalf("failed = %v, want [netflix]", failed)
	}
}

func TestGatherShoeFailsWhenAllSourcesFail(t *testing.T) {
	a := &fakeSource{id: SourceJellyfin, err: errors.New("down")}
	b := &fakeSource{id: SourceNetflix, err: errors.New("down")}

	_, failed, err := gatherShoe(context.Background(), []MovieSource{a, b}, Filters{})
	if !errors.Is(err, errAllSourcesFailed) {
		t.Fatalf("expected errAllSourcesFailed, got %v", err)
	}
	if len(failed) != 2 {
		t.Fatalf("expected both sources reported failed, got %v", failed)
	}
}
