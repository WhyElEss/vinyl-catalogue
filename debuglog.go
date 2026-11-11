package main

// TEST COPIES ONLY. With DEBUG_LOG=1 the page in ?debug=wheel mode posts
// its input log here, and it is kept as debug-wheel-log.json in the data
// directory. Without DEBUG_LOG the route does not exist.

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (a *App) debugRoutes(mux *http.ServeMux) {
	if os.Getenv("DEBUG_LOG") != "1" {
		return
	}
	mux.HandleFunc("POST /debug/wheel-log", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
		if err != nil || len(body) == 0 {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		p := filepath.Join(a.cfg.DataDir, "debug-wheel-log.json")
		if err := os.WriteFile(p+".tmp", body, 0o644); err != nil || os.Rename(p+".tmp", p) != nil {
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
