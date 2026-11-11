package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// syncLoop reads the collection at start and refreshes everything once per
// SyncEvery (a day by default):
//   - at start: the collection list, plus whatever is still missing (covers,
//     release details, original years), so a restart shows new records;
//   - when SyncEvery has passed since the last full sync: the list and every
//     release again, so edited tracklists and newly uploaded covers arrive.
func (a *App) syncLoop(ctx context.Context) {
	var last time.Time
	a.store.view(func(d *Data) { last = d.FullSyncAt })
	full := time.Since(last) >= a.cfg.SyncEvery
	for {
		if err := a.syncOnce(ctx, full); err != nil && ctx.Err() == nil {
			log.Printf("sync: %v", err)
		}
		a.store.view(func(d *Data) { last = d.FullSyncAt })
		wait := time.Until(last.Add(a.cfg.SyncEvery))
		if wait < time.Minute {
			wait = time.Hour // a full sync was due and failed: try again in an hour
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		full = true
	}
}

// syncOnce reads the owner and the collection list and publishes them, then
// fills in covers, release details and original years. With full set it
// re-reads every release's details (and so its current primary image).
func (a *App) syncOnce(ctx context.Context, full bool) error {
	start := time.Now()
	user, err := a.discogs.user(ctx, a.cfg.DiscogsUser)
	if err != nil {
		return fmt.Errorf("user: %w", err)
	}
	// The owner's "Sort as" field, if there is one (by name, any case). A
	// failure here only means no custom order this time.
	sortField := 0
	if f, err := a.discogs.collectionFields(ctx, a.cfg.DiscogsUser); err != nil {
		log.Printf("sync: collection fields: %v", err)
	} else {
		for _, x := range f.Fields {
			if strings.EqualFold(strings.TrimSpace(x.Name), sortAsField) {
				sortField = x.ID
			}
		}
	}
	var releases []Release
	for page, pages := 1, 1; page <= pages; page++ {
		p, err := a.discogs.collectionPage(ctx, a.cfg.DiscogsUser, page)
		if err != nil {
			return fmt.Errorf("collection page %d: %w", page, err)
		}
		pages = p.Pagination.Pages
		for _, r := range p.Releases {
			rel := releaseFromAPI(r.ID, r.InstanceID, r.Basic.MasterID, r.Basic.Title, r.Basic.CoverImage,
				r.Basic.Year, r.Basic.Artists, r.Basic.Formats, r.Basic.Labels, r.Basic.Genres, r.Basic.Styles)
			for _, n := range r.Notes {
				if sortField != 0 && n.FieldID == sortField {
					rel.SortAs = strings.TrimSpace(n.Value)
				}
			}
			releases = append(releases, rel)
		}
	}

	var before int
	if err := a.store.update(func(d *Data) {
		before = len(d.Releases)
		d.Owner = Owner{Username: user.Username, Name: user.Name}
		d.Releases = releases
		d.SyncedAt = time.Now().UTC()
	}); err != nil {
		return err
	}
	if before != len(releases) {
		log.Printf("sync: collection has %d releases (was %d)", len(releases), before)
	}

	// Covers first, so a new record has its picture within a minute; then
	// again after the details, which may name a newer primary image.
	a.fetchCovers(ctx, releases)
	n, err := a.fetchDetails(ctx, releases, full)
	if err != nil {
		return err
	}
	replaced := a.fetchCovers(ctx, releases)
	if full {
		if err := a.store.update(func(d *Data) { d.FullSyncAt = time.Now().UTC() }); err != nil {
			return err
		}
		log.Printf("sync: full refresh of %d releases done in %s, %d covers replaced", n, time.Since(start).Round(time.Second), replaced)
	} else if n > 0 {
		log.Printf("sync: read details for %d releases in %s", n, time.Since(start).Round(time.Second))
	}
	return nil
}

var disambiguation = regexp.MustCompile(`\s+\(\d+\)$`)

// artistName drops the Discogs disambiguation suffix: "Wings (2)" → "Wings".
func artistName(s string) string { return disambiguation.ReplaceAllString(strings.TrimSpace(s), "") }

func convertArtists(in []apiArtist) []Artist {
	out := make([]Artist, 0, len(in))
	for _, a := range in {
		name := artistName(a.Name)
		shown := strings.TrimSpace(a.ANV)
		if shown == "" {
			shown = name
		}
		out = append(out, Artist{Name: shown, Sort: name, Join: strings.TrimSpace(a.Join), ID: a.ID})
	}
	return out
}

func releaseFromAPI(id, instance, master int64, title, cover string, year int,
	artists []apiArtist, formats []apiFormat, labels []apiLabel, genres, styles []string) Release {
	r := Release{Instance: instance, ID: id, MasterID: master, Title: strings.TrimSpace(title), Year: year,
		Genres: genres, Styles: styles, CoverURL: cover, Artists: convertArtists(artists)}
	for _, f := range formats {
		r.Formats = append(r.Formats, Format(f))
	}
	seen := map[string]bool{}
	for _, l := range labels {
		key := l.Name + "\x00" + l.Catno
		if seen[key] {
			continue
		}
		seen[key] = true
		r.Labels = append(r.Labels, Label{Name: artistName(l.Name), Catno: l.Catno})
	}
	return r
}

func convertTracks(in []apiTrack) []Track {
	var out []Track
	for _, t := range in {
		tr := Track{Position: t.Position, Title: t.Title, Duration: t.Duration, Heading: t.Type == "heading"}
		if len(t.Artists) > 0 {
			tr.Artists = joinArtists(convertArtists(t.Artists))
		}
		if len(t.SubTracks) > 0 {
			tr.Sub = convertTracks(t.SubTracks)
		}
		out = append(out, tr)
	}
	return out
}

// primaryImage is the release's primary image, else its first one.
func primaryImage(rel apiRelease) string {
	for _, im := range rel.Images {
		if im.Type == "primary" && im.URI != "" {
			return im.URI
		}
	}
	if len(rel.Images) > 0 {
		return rel.Images[0].URI
	}
	return ""
}

// coverSource is the Discogs image a release's cover comes from: the
// release's primary image once its details are read, else the list's
// cover_image.
func coverSource(d *Data, r *Release) string {
	if det, ok := d.Details[r.ID]; ok && det.Cover != "" {
		return det.Cover
	}
	if strings.Contains(r.CoverURL, "spacer.gif") {
		return ""
	}
	return r.CoverURL
}

// coverPath is where a release's cover is stored.
func (a *App) coverPath(id int64) string {
	return filepath.Join(a.coversDir, strconv.FormatInt(id, 10)+".jpg")
}

// fetchCovers downloads covers that are missing or whose Discogs image
// changed, replacing the old file. The source URL is kept next to the file.
// It returns how many existing covers were replaced.
func (a *App) fetchCovers(ctx context.Context, releases []Release) (replaced int) {
	type want struct {
		id  int64
		src string
	}
	var wants []want
	seen := map[int64]bool{}
	a.store.view(func(d *Data) {
		for i := range releases {
			r := &releases[i]
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			if src := coverSource(d, r); src != "" {
				wants = append(wants, want{r.ID, src})
			}
		}
	})
	var failed int
	for _, w := range wants {
		if ctx.Err() != nil {
			return replaced
		}
		srcFile := a.coverPath(w.id) + ".src"
		old, err := os.ReadFile(srcFile)
		if err == nil && string(old) == w.src {
			continue
		}
		if err := a.downloadCover(ctx, w.src, a.coverPath(w.id)); err != nil {
			failed++
			if failed <= 3 {
				log.Printf("cover %d: %v", w.id, err)
			}
			continue
		}
		if len(old) > 0 {
			replaced++
		}
		os.WriteFile(srcFile, []byte(w.src), 0o644)
	}
	if failed > 3 {
		log.Printf("covers: %d downloads failed", failed)
	}
	return replaced
}

func (a *App) downloadCover(ctx context.Context, src, dst string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", a.discogs.ua)
	resp, err := a.imgClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/jpeg") {
		return fmt.Errorf("unexpected content type %q", ct)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return err
	}
	if len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
		return errors.New("not a JPEG")
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// fetchDetails reads release details (all of them when full, else the
// missing ones) and the original years still unknown, in list order (newest
// first), saving every 10 releases.
func (a *App) fetchDetails(ctx context.Context, releases []Release, full bool) (int, error) {
	type job struct {
		id, master int64
	}
	var jobs []job
	seen := map[int64]bool{}
	a.store.view(func(d *Data) {
		for _, r := range releases {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			_, haveDetails := d.Details[r.ID]
			_, haveMaster := d.MasterYears[r.MasterID]
			needMaster := r.MasterID != 0 && !haveMaster
			switch {
			case full || !haveDetails:
				jobs = append(jobs, job{r.ID, r.MasterID})
			case needMaster:
				jobs = append(jobs, job{0, r.MasterID})
			}
		}
	})
	if len(jobs) == 0 {
		return 0, nil
	}
	read := 0
	// Whatever was read before an error or shutdown is kept.
	defer a.store.update(func(*Data) {})
	for i, j := range jobs {
		if j.id != 0 {
			rel, err := a.discogs.release(ctx, j.id)
			switch {
			case errors.Is(err, errNotFound):
				rel = apiRelease{}
			case err != nil:
				return read, fmt.Errorf("release %d: %w", j.id, err)
			}
			det := &Details{Country: rel.Country, Released: rel.Released, Notes: strings.TrimSpace(rel.Notes),
				Tracklist: convertTracks(rel.Tracklist), Cover: primaryImage(rel), ReadAt: time.Now().UTC()}
			a.store.mu.Lock()
			a.store.data.Details[j.id] = det
			a.store.mu.Unlock()
			read++
		}
		var haveMaster bool
		a.store.view(func(d *Data) { _, haveMaster = d.MasterYears[j.master] })
		if j.master != 0 && !haveMaster {
			m, err := a.discogs.master(ctx, j.master)
			if err != nil && !errors.Is(err, errNotFound) {
				return read, fmt.Errorf("master %d: %w", j.master, err)
			}
			a.store.mu.Lock()
			a.store.data.MasterYears[j.master] = m.Year
			a.store.mu.Unlock()
		}
		if (i+1)%10 == 0 {
			if err := a.store.update(func(*Data) {}); err != nil {
				return read, err
			}
		}
	}
	return read, nil
}
