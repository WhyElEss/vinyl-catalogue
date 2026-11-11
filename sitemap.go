package main

import (
	"bytes"
	"encoding/xml"
	"net/http"
)

// handleSitemap lists the collection page and every record's own page for
// search engines (robots.txt points here).
func (a *App) handleSitemap(w http.ResponseWriter, r *http.Request) {
	origin := a.origin(r)
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	loc := func(path string) {
		buf.WriteString("<url><loc>")
		xml.EscapeText(&buf, []byte(origin+path))
		buf.WriteString("</loc></url>\n")
	}
	loc("/")
	a.store.view(func(d *Data) {
		for i := range d.Releases {
			loc(releasePath(&d.Releases[i]))
		}
	})
	buf.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(buf.Bytes())
}
