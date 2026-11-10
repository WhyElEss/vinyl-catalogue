package main

import (
	"image/jpeg"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// releaseView is what the release templates show.
type releaseView struct {
	Instance     int64
	ID           int64
	Path         string
	Artist       string
	Title        string
	Cover        string
	OriginalYear int
	Year         int
	Released     string
	Country      string
	Tags         []string
	Formats      []string
	Labels       []string
	Catnos       []string
	Tracklist    []Track
	Notes        []string
	HasDetails   bool
	DiscogsURL   string
}

type releasePage struct {
	base
	R releaseView
}

var slugJunk = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func slug(s string) string {
	return strings.Trim(slugJunk.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func releasePath(r *Release) string {
	p := "/release/" + strconv.FormatInt(r.Instance, 10)
	if s := slug(r.artistLine() + " " + r.Title); s != "" {
		p += "/" + s
	}
	return p
}

// handleRelease shows one record. With ?part=1 it answers only the inner
// article, which the page script puts into its overlay.
func (a *App) handleRelease(w http.ResponseWriter, r *http.Request) {
	inst, err := strconv.ParseInt(r.PathValue("instance"), 10, 64)
	var v releaseView
	found := false
	if err == nil {
		a.store.view(func(d *Data) {
			for i := range d.Releases {
				rel := &d.Releases[i]
				if rel.Instance != inst {
					continue
				}
				found = true
				v = a.makeReleaseView(d, rel)
				return
			}
		})
	}
	b := a.base(r)
	if !found {
		a.render(w, http.StatusNotFound, "message", "layout", messagePage{base: b, Text: t("not_found")})
		return
	}
	b.Title = v.Artist + " – " + v.Title
	b.OGTitle = b.Title
	b.OGURL = b.Origin + v.Path
	// "1985 · 1 × Vinyl, LP, Album · From Yuri Stanislavski's record collection"
	var desc []string
	if y := v.OriginalYear; y > 0 {
		desc = append(desc, strconv.Itoa(y))
	} else if v.Year > 0 {
		desc = append(desc, strconv.Itoa(v.Year))
	}
	if len(v.Formats) > 0 {
		desc = append(desc, v.Formats[0])
	}
	desc = append(desc, "From "+b.SiteName)
	b.Description = strings.Join(desc, " · ")
	if v.Cover != "" {
		if w, h, ok := a.coverSize(v.ID); ok {
			b.OGImage = b.Origin + v.Cover
			b.OGImageType, b.OGImageW, b.OGImageH = "image/jpeg", w, h
			b.TwitterCard = "summary"
		}
	}
	p := releasePage{base: b, R: v}
	if r.URL.Query().Get("part") == "1" {
		a.render(w, http.StatusOK, "release", "release_body", p)
		return
	}
	a.render(w, http.StatusOK, "release", "layout", p)
}

func (a *App) makeReleaseView(d *Data, r *Release) releaseView {
	v := releaseView{
		Instance:     r.Instance,
		ID:           r.ID,
		Path:         releasePath(r),
		Artist:       r.artistLine(),
		Title:        r.Title,
		Cover:        a.coverURL(d, r),
		OriginalYear: originalYear(d, r),
		Year:         r.Year,
		Tags:         r.tags(),
		Formats:      r.formatLines(),
		DiscogsURL:   "https://www.discogs.com/release/" + strconv.FormatInt(r.ID, 10),
	}
	v.Labels, v.Catnos = labelsAndNumbers(r.Labels)
	if det, ok := d.Details[r.ID]; ok {
		v.HasDetails = true
		v.Country = det.Country
		v.Released = formatReleased(det.Released)
		v.Tracklist = det.Tracklist
		v.Notes = cleanNotes(det.Notes)
	}
	return v
}

type messagePage struct {
	base
	Text string
}

// labelsAndNumbers lists label names and catalogue numbers apart, each once:
// Discogs repeats a label for every way its number is printed.
func labelsAndNumbers(ls []Label) (names, numbers []string) {
	seenName, seenNo := map[string]bool{}, map[string]bool{}
	for _, l := range ls {
		if l.Name != "" && !seenName[l.Name] {
			seenName[l.Name] = true
			names = append(names, l.Name)
		}
		if c := strings.TrimSpace(l.Catno); c != "" && !strings.EqualFold(c, "none") && !seenNo[c] {
			seenNo[c] = true
			numbers = append(numbers, c)
		}
	}
	return names, numbers
}

// coverSize reads a stored cover's pixel size for the link preview.
func (a *App) coverSize(id int64) (w, h int, ok bool) {
	f, err := os.Open(a.coverPath(id))
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, err := jpeg.DecodeConfig(f)
	if err != nil {
		return 0, 0, false
	}
	return cfg.Width, cfg.Height, true
}
