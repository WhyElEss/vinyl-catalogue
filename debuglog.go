package main

// TEST COPIES ONLY. With DEBUG_LOG=1 the page in ?debug=wheel mode posts
// its input log here, and it is kept as debug-wheel-log.json in the data
// directory — or debug-wheel-log-<b>.json when the page posts with
// ?b=<digits> (a page opened with b=<digits> in its address does), so pages
// left open in other tabs (they post every 2 s) do not overwrite the one
// being measured. Without DEBUG_LOG the
// route does not exist.

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
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
		name := "debug-wheel-log.json"
		if n, err := strconv.Atoi(r.URL.Query().Get("b")); err == nil && n >= 0 {
			name = "debug-wheel-log-" + strconv.Itoa(n) + ".json"
		}
		p := filepath.Join(a.cfg.DataDir, name)
		if err := os.WriteFile(p+".tmp", body, 0o644); err != nil || os.Rename(p+".tmp", p) != nil {
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
