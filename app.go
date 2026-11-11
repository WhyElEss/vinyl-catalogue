package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed templates static
var assets embed.FS

type App struct {
	cfg       Config
	store     *Store
	discogs   *discogsAPI
	imgClient *http.Client
	coversDir string
	tpl       map[string]*template.Template
}

func newApp(cfg Config) (*App, error) {
	st, err := openStore(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	covers := filepath.Join(cfg.DataDir, "covers")
	if err := os.MkdirAll(covers, 0o755); err != nil {
		return nil, err
	}
	tpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:       cfg,
		store:     st,
		discogs:   newDiscogsAPI("https://api.discogs.com", cfg.DiscogsToken),
		imgClient: &http.Client{Timeout: 30 * time.Second},
		coversDir: covers,
		tpl:       tpl,
	}, nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", cacheFor(365*24*time.Hour, http.StripPrefix("/static/", http.FileServerFS(static))))
	for _, name := range []string{"favicon.ico", "apple-touch-icon.png"} {
		mux.Handle("GET /"+name, cacheFor(24*time.Hour, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, static, name)
		})))
	}
	// Search engines and link-preview crawlers are welcome (owner,
	// 2026-11-11); the sitemap lists every record's page.
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", a.origin(r))
	})
	mux.HandleFunc("GET /sitemap.xml", a.handleSitemap)
	a.debugRoutes(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /cover/{file}", a.handleCover)
	mux.HandleFunc("GET /{$}", a.handleIndex)
	mux.HandleFunc("GET /release/{instance}", a.handleRelease)
	mux.HandleFunc("GET /release/{instance}/{slug}", a.handleRelease)
	return a.secure(mux)
}

func cacheFor(d time.Duration, h http.Handler) http.Handler {
	maxAge := strconv.Itoa(int(d.Seconds()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+maxAge)
		h.ServeHTTP(w, r)
	})
}

// secure sets the headers every response shares. The site has no forms and
// no cookies.
func (a *App) secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		// HTTPS only from now on for this host (not its siblings), and no
		// window opened from elsewhere shares a browsing context with ours.
		hd.Set("Strict-Transport-Security", "max-age=31536000")
		hd.Set("Cross-Origin-Opener-Policy", "same-origin")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		hd.Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		h.ServeHTTP(w, r)
	})
}

func (a *App) origin(r *http.Request) string {
	if a.cfg.BaseURL != "" {
		return a.cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

var templateFuncs = template.FuncMap{
	"t": t,
	"tf": func(key string, args ...any) string {
		return fmt.Sprintf(t(key), args...)
	},
	"count": releasesCount,
}

func loadTemplates() (map[string]*template.Template, error) {
	m := map[string]*template.Template{}
	for _, p := range []string{"index", "release", "message"} {
		tp, err := template.New(p).Funcs(templateFuncs).ParseFS(assets, "templates/layout.html", "templates/release_body.html", "templates/"+p+".html")
		if err != nil {
			return nil, err
		}
		m[p] = tp
	}
	return m, nil
}

// base is embedded in every page's data and read by the layout.
type base struct {
	Title     string // <title>
	OwnerName string
	Count     int
	Version   string
	Path      string
	// Link previews, always in English (as on the wishlist): the collection
	// card by default, a record's own cover on its page.
	Origin      string
	OGURL       string
	SiteName    string
	OGTitle     string
	Description string
	OGImage     string // absolute URL
	OGImageType string
	OGImageW    int
	OGImageH    int
	TwitterCard string // summary_large_image for the card, summary for a square cover
}

// The link-preview card (static/og.png).
const (
	ogCardW = 1200
	ogCardH = 630
)

func (a *App) base(r *http.Request) base {
	b := base{Version: version, Path: r.URL.Path, Origin: a.origin(r)}
	a.store.view(func(d *Data) {
		b.OwnerName = d.Owner.Name
		if b.OwnerName == "" {
			b.OwnerName = d.Owner.Username
		}
		b.Count = len(d.Releases)
	})
	b.Title = t("collection")
	if b.OwnerName != "" {
		b.Title = fmt.Sprintf(t("collection_of"), b.OwnerName)
	}
	b.SiteName = b.Title
	b.OGTitle = b.SiteName
	b.OGURL = b.Origin + "/"
	b.Description = releasesCount(b.Count) + ". Dig through the crates."
	b.OGImage = b.Origin + "/static/og.png?v=" + version
	b.OGImageType, b.OGImageW, b.OGImageH = "image/png", ogCardW, ogCardH
	b.TwitterCard = "summary_large_image"
	return b
}

func (a *App) render(w http.ResponseWriter, status int, name, tmpl string, data any) {
	var buf bytes.Buffer
	if err := a.tpl[name].ExecuteTemplate(&buf, tmpl, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

type indexPage struct {
	base
	Releases []clientRelease
	Data     template.JS // JSON for the page script, inside a type="application/json" block
	Texts    template.JS
}

// clientTexts are the strings the page script needs.
var clientTextKeys = []string{"of", "nothing", "prev", "next",
	"close", "other", "prev_record", "next_record", "enter_stack", "order_asc", "order_desc", "various"}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	p := indexPage{base: a.base(r)}
	a.store.view(func(d *Data) {
		rs := make([]*Release, len(d.Releases))
		for i := range d.Releases {
			rs[i] = &d.Releases[i]
		}
		byArtist(d, rs)
		all := a.clientReleases(d)
		pos := map[int64]int{}
		for i, c := range all {
			pos[c.I] = i
		}
		for i, r := range rs {
			c := all[pos[r.Instance]]
			c.P = i
			p.Releases = append(p.Releases, c)
		}
	})
	data, _ := json.Marshal(p.Releases)
	p.Data = template.JS(data)
	tx := map[string]string{}
	for _, k := range clientTextKeys {
		tx[k] = t(k)
	}
	tx["one"], tx["many"] = "release", "releases"
	txt, _ := json.Marshal(tx)
	p.Texts = template.JS(txt)
	a.render(w, http.StatusOK, "index", "layout", p)
}

// coverURL is the site's own URL for a release's cover, "" without one. The
// query changes when the image is replaced or re-encoded (it hashes the
// source URL and the file size), so browsers may cache for long.
func (a *App) coverURL(d *Data, r *Release) string {
	// The file's own source, which may lag behind coverSource until the
	// download is done.
	src, err := os.ReadFile(a.coverPath(r.ID) + ".src")
	if err != nil {
		return ""
	}
	size := ""
	if st, err := os.Stat(a.coverPath(r.ID)); err == nil {
		size = strconv.FormatInt(st.Size(), 10)
	}
	return "/cover/" + strconv.FormatInt(r.ID, 10) + ".jpg?v=" + shortHash(string(src)+"|"+size)
}

func shortHash(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return strconv.FormatUint(uint64(h), 36)
}

func (a *App) handleCover(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	id, ok := strings.CutSuffix(name, ".jpg")
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(a.coversDir, id+".jpg"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", st.ModTime(), f)
}
