package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eiladin/movie-night-showdown/server"
)

//go:embed all:web/dist
var webDist embed.FS

// Injected at build time via -ldflags (see scripts/publish.sh).
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		healthcheck()
	}

	cfg := server.LoadConfig()
	log.Printf("config: %s", cfg)
	log.Printf("movie-night-showdown %s (commit %s)", version, commit)

	dist, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	s := server.New(cfg)
	s.SetStatic(dist)
	s.SetBuildInfo(version, commit)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: s.Handler(),
	}

	go func() {
		log.Printf("movie-night-showdown listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

// healthcheck pings the local /healthz endpoint and exits 0 if healthy,
// 1 otherwise. Invoked as `showdown -healthcheck` by the deployment's
// container healthcheck, since the distroless image has no curl/wget/shell.
func healthcheck() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // must match server.LoadConfig's default
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		os.Exit(1)
	}
	// Read the status before closing, and close explicitly rather than with
	// defer: os.Exit does not run deferred calls.
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}
