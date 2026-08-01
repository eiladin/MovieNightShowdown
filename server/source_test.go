package server

import "testing"

func TestMergeMoviesUnionsAvailability(t *testing.T) {
	jf := []Movie{
		{ID: "tmdb:603", Title: "The Matrix", Availability: []Availability{{Source: SourceJellyfin}}},
		{ID: "jf:abc", Title: "Home Video", Availability: []Availability{{Source: SourceJellyfin}}},
	}
	netflix := []Movie{
		{ID: "tmdb:603", Title: "The Matrix", Availability: []Availability{{Source: SourceNetflix}}},
		{ID: "tmdb:78", Title: "Blade Runner", Availability: []Availability{{Source: SourceNetflix}}},
	}

	got := MergeMovies(jf, netflix)

	if len(got) != 3 {
		t.Fatalf("expected 3 merged movies, got %d", len(got))
	}
	var matrix *Movie
	for i := range got {
		if got[i].ID == "tmdb:603" {
			matrix = &got[i]
		}
	}
	if matrix == nil {
		t.Fatalf("merged set is missing tmdb:603")
	}
	if len(matrix.Availability) != 2 {
		t.Fatalf("expected 2 availability entries for tmdb:603, got %d", len(matrix.Availability))
	}
	if matrix.Availability[0].Source != SourceJellyfin || matrix.Availability[1].Source != SourceNetflix {
		t.Fatalf("unexpected availability order: %+v", matrix.Availability)
	}
}

func TestMergeMoviesDoesNotDuplicateSameSource(t *testing.T) {
	a := []Movie{{ID: "tmdb:603", Availability: []Availability{{Source: SourceNetflix}}}}
	b := []Movie{{ID: "tmdb:603", Availability: []Availability{{Source: SourceNetflix}}}}

	got := MergeMovies(a, b)

	if len(got) != 1 {
		t.Fatalf("expected 1 merged movie, got %d", len(got))
	}
	if len(got[0].Availability) != 1 {
		t.Fatalf("expected 1 availability entry, got %d", len(got[0].Availability))
	}
}

func TestMergeMoviesEmptyInput(t *testing.T) {
	got := MergeMovies()
	if got == nil {
		t.Fatalf("MergeMovies() returned nil; expected an empty non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}
