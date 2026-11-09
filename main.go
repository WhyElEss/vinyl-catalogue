// Command vinyl-catalogue shows one Discogs user's record collection as a
// read-only website. The collection is read from the Discogs API, kept in one
// JSON file under DATA_DIR, and the cover images are stored next to it.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var version = "dev"

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApp(cfg)
	if err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           app.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.syncLoop(ctx)
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()

	log.Printf("vinyl-catalogue %s listening on %s, data in %s, Discogs user %s", version, cfg.Listen, cfg.DataDir, cfg.DiscogsUser)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
