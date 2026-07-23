package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/eiladin/movie-night-showdown/server"
)

//go:embed all:web/dist
var webDist embed.FS

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dist, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	s := server.New()
	s.SetStatic(dist)

	log.Printf("movie-night-showdown listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, s.Handler()))
}
