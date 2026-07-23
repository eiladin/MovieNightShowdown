// Command mock-jellyfin is a self-contained stand-in for a real Jellyfin
// server, used only by the screenshot-generation pipeline (see
// scripts/screenshots/README.md). It implements just the three endpoints
// server.JellyfinClient calls (see server/jellyfin.go), backed by a
// hardcoded list of fictional movies and placeholder poster art. No real
// Jellyfin server, network access, or personal data is involved.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

// fixtureMovie is one hardcoded, fictional movie served by this mock. Field
// names mirror the Jellyfin /Items item shape consumed by
// server.JellyfinClient.Movies (see server/jellyfin.go).
type fixtureMovie struct {
	ID              string
	Name            string
	ProductionYear  int
	Genres          []string
	Overview        string
	RuntimeMinutes  int
	CommunityRating float64
	OfficialRating  string
	ImageTag        string
}

// fixtureMovies is the invented movie catalog this mock serves. Titles,
// people, and artwork are fictional; none of this represents real films.
var fixtureMovies = []fixtureMovie{
	{
		ID: "movie-01", Name: "The Last Signal", ProductionYear: 2019,
		Genres: []string{"Science Fiction", "Thriller"},
		Overview: "A deep-space relay operator picks up a transmission that " +
			"shouldn't exist.",
		RuntimeMinutes: 128, CommunityRating: 7.8, OfficialRating: "PG-13",
		ImageTag: "poster01v1",
	},
	{
		ID: "movie-02", Name: "Paper Moons", ProductionYear: 1994,
		Genres:         []string{"Drama", "Romance"},
		Overview:       "Two pen pals finally meet, twenty years and one continent too late.",
		RuntimeMinutes: 104, CommunityRating: 7.2, OfficialRating: "PG",
		ImageTag: "poster02v1",
	},
	{
		ID: "movie-03", Name: "Neon Alley Cats", ProductionYear: 2021,
		Genres:         []string{"Action", "Comedy"},
		Overview:       "A washed-up getaway driver reluctantly trains her replacement.",
		RuntimeMinutes: 112, CommunityRating: 6.9, OfficialRating: "R",
		ImageTag: "poster03v1",
	},
	{
		ID: "movie-04", Name: "Whispering Pines", ProductionYear: 2003,
		Genres:         []string{"Horror", "Thriller"},
		Overview:       "A ranger station's night shift log fills in for the missing crew.",
		RuntimeMinutes: 97, CommunityRating: 6.4, OfficialRating: "R",
		ImageTag: "poster04v1",
	},
	{
		ID: "movie-05", Name: "Captain Fizzbucket's Grand Voyage", ProductionYear: 1998,
		Genres:         []string{"Family", "Adventure"},
		Overview:       "A tugboat captain and his crew of misfit animals chase a legend.",
		RuntimeMinutes: 89, CommunityRating: 7.1, OfficialRating: "G",
		ImageTag: "poster05v1",
	},
	{
		ID: "movie-06", Name: "Midnight Cartographers", ProductionYear: 2016,
		Genres:         []string{"Mystery", "Drama"},
		Overview:       "A surveyor's abandoned maps redraw a town's history overnight.",
		RuntimeMinutes: 118, CommunityRating: 7.6, OfficialRating: "PG-13",
		ImageTag: "poster06v1",
	},
	{
		ID: "movie-07", Name: "The Clockwork Hearts", ProductionYear: 2011,
		Genres:         []string{"Romance", "Science Fiction"},
		Overview:       "Two rival inventors discover their automatons have fallen in love.",
		RuntimeMinutes: 105, CommunityRating: 7.0, OfficialRating: "PG",
		ImageTag: "poster07v1",
	},
	{
		ID: "movie-08", Name: "Iron Harvest Blues", ProductionYear: 1992,
		Genres:         []string{"Drama", "Western"},
		Overview:       "A failing homestead becomes the last stop on a bounty hunter's route.",
		RuntimeMinutes: 133, CommunityRating: 8.1, OfficialRating: "PG-13",
		ImageTag: "poster08v1",
	},
	{
		ID: "movie-09", Name: "Static & Fury", ProductionYear: 2023,
		Genres:         []string{"Action", "Thriller"},
		Overview:       "A disgraced stunt coordinator has one night to clear her name.",
		RuntimeMinutes: 121, CommunityRating: 7.4, OfficialRating: "R",
		ImageTag: "poster09v1",
	},
	{
		ID: "movie-10", Name: "The Quiet Algorithm", ProductionYear: 2020,
		Genres:         []string{"Science Fiction", "Drama"},
		Overview:       "A retiring engineer teaches her replacement the system's one secret.",
		RuntimeMinutes: 110, CommunityRating: 8.3, OfficialRating: "PG-13",
		ImageTag: "poster10v1",
	},
	{
		ID: "movie-11", Name: "Grandma's Rocket", ProductionYear: 2001,
		Genres:         []string{"Comedy", "Family"},
		Overview:       "A retiree builds a backyard rocket to win a bet with her grandson.",
		RuntimeMinutes: 95, CommunityRating: 6.8, OfficialRating: "PG",
		ImageTag: "poster11v1",
	},
	{
		ID: "movie-12", Name: "Dust and Starlight", ProductionYear: 2018,
		Genres:         []string{"Adventure", "Drama"},
		Overview:       "A caravan of salvagers crosses a dry seabed toward a rumored oasis.",
		RuntimeMinutes: 141, CommunityRating: 8.7, OfficialRating: "PG-13",
		ImageTag: "poster12v1",
	},
	{
		ID: "movie-13", Name: "Bone Orchard Nights", ProductionYear: 2007,
		Genres:         []string{"Horror"},
		Overview:       "An orchard's new caretaker learns why the last three quit.",
		RuntimeMinutes: 102, CommunityRating: 6.1, OfficialRating: "TV-MA",
		ImageTag: "poster13v1",
	},
	{
		ID: "movie-14", Name: "The Umbrella Conspiracy", ProductionYear: 1996,
		Genres:         []string{"Thriller", "Mystery"},
		Overview:       "A weather forecaster notices the storms only hit one street.",
		RuntimeMinutes: 124, CommunityRating: 7.3, OfficialRating: "R",
		ImageTag: "poster14v1",
	},
	{
		ID: "movie-15", Name: "Sunday Static", ProductionYear: 2022,
		Genres:         []string{"Comedy", "Drama"},
		Overview:       "Three estranged siblings inherit their father's failing radio station.",
		RuntimeMinutes: 99, CommunityRating: 6.6, OfficialRating: "TV-MA",
		ImageTag: "poster15v1",
	},
}

// jellyfinItem mirrors the subset of a Jellyfin /Items entry that
// server.JellyfinClient.Movies decodes (see server/jellyfin.go).
type jellyfinItem struct {
	ID              string            `json:"Id"`
	Name            string            `json:"Name"`
	ProductionYear  int               `json:"ProductionYear"`
	Genres          []string          `json:"Genres"`
	Overview        string            `json:"Overview"`
	RunTimeTicks    int64             `json:"RunTimeTicks"`
	CommunityRating float64           `json:"CommunityRating"`
	OfficialRating  string            `json:"OfficialRating"`
	ImageTags       map[string]string `json:"ImageTags"`
}

type itemsResponse struct {
	Items            []jellyfinItem `json:"Items"`
	TotalRecordCount int            `json:"TotalRecordCount"`
}

// fakeTotalRecordCount is a fixed, plausible library size reported
// regardless of the requested filters (see server/filters.go for the
// filters this mock intentionally ignores).
const fakeTotalRecordCount = 137

func toItem(m fixtureMovie) jellyfinItem {
	return jellyfinItem{
		ID:              m.ID,
		Name:            m.Name,
		ProductionYear:  m.ProductionYear,
		Genres:          m.Genres,
		Overview:        m.Overview,
		RunTimeTicks:    int64(m.RuntimeMinutes) * 60 * 10_000_000,
		CommunityRating: m.CommunityRating,
		OfficialRating:  m.OfficialRating,
		ImageTags:       map[string]string{"Primary": m.ImageTag},
	}
}

// handleItems serves GET /Items. It ignores the filter query params (genres,
// years, ratings, etc.) and always returns the full fixture catalog — see
// scripts/screenshots/README.md for why that's sufficient for screenshots.
func handleItems(w http.ResponseWriter, r *http.Request) {
	items := make([]jellyfinItem, 0, len(fixtureMovies))
	for _, m := range fixtureMovies {
		items = append(items, toItem(m))
	}
	writeJSON(w, itemsResponse{Items: items, TotalRecordCount: fakeTotalRecordCount})
}

// handleFilters serves GET /Items/Filters?IncludeItemTypes=Movie: the union
// of genres and official ratings across the fixture catalog.
func handleFilters(w http.ResponseWriter, r *http.Request) {
	genreSet := map[string]struct{}{}
	ratingSet := map[string]struct{}{}
	for _, m := range fixtureMovies {
		for _, g := range m.Genres {
			genreSet[g] = struct{}{}
		}
		ratingSet[m.OfficialRating] = struct{}{}
	}

	writeJSON(w, struct {
		Genres          []string `json:"Genres"`
		OfficialRatings []string `json:"OfficialRatings"`
	}{
		Genres:          sortedKeys(genreSet),
		OfficialRatings: sortedKeys(ratingSet),
	})
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// postersDir is where fixture poster PNGs live, keyed by movie ID
// (<id>.png). Overridable via MOCK_POSTERS_DIR for callers not running from
// the repo root.
func postersDir() string {
	if dir := os.Getenv("MOCK_POSTERS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join("scripts", "screenshots", "fixtures", "posters")
}

// handlePosterImage serves GET /Items/{id}/Images/Primary, matching the path
// server.JellyfinClient.fetchPoster requests (see server/jellyfin.go).
func handlePosterImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := filepath.Join(postersDir(), id+".png")
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("mock-jellyfin: encode response: %v", err)
	}
}

func main() {
	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = "8099"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /Items", handleItems)
	mux.HandleFunc("GET /Items/Filters", handleFilters)
	mux.HandleFunc("GET /Items/{id}/Images/Primary", handlePosterImage)

	log.Printf("mock-jellyfin: %d fictional movies, posters from %s", len(fixtureMovies), postersDir())
	log.Printf("mock-jellyfin: listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("mock-jellyfin: %v", err)
	}
}
