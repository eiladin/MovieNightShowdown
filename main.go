package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"github.com/eiladin/movie-night-showdown/server"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	cfg := server.LoadConfig()
	log.Printf("config: %s", cfg)

	dist, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	s := server.New(cfg)
	s.SetStatic(dist)

	log.Printf("movie-night-showdown listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, s.Handler()))
}
