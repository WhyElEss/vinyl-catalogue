package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// discogsAPI reads the public parts of api.discogs.com. Discogs counts
// requests in a moving 60-second window: 25 a minute without a token, 60 with
// one. Requests are spaced so a full minute never goes over the limit.
type discogsAPI struct {
	base   string // https://api.discogs.com, a test server in tests
	token  string
	ua     string
	client *http.Client

	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

func newDiscogsAPI(base, token string) *discogsAPI {
	gap := 2500 * time.Millisecond // 24 a minute
	if token != "" {
		gap = 1050 * time.Millisecond // 57 a minute
	}
	return &discogsAPI{
		base:   base,
		token:  token,
		ua:     "VinylCatalogue/" + version,
		client: &http.Client{Timeout: 30 * time.Second},
		gap:    gap,
	}
}

// errNotFound is a 404 from the API: the release or master was deleted or
// merged on Discogs.
var errNotFound = errors.New("not found on Discogs")

// wait blocks until the next request may go out.
func (d *discogsAPI) wait(ctx context.Context) error {
	d.mu.Lock()
	now := time.Now()
	at := d.next
	if at.Before(now) {
		at = now
	}
	d.next = at.Add(d.gap)
	d.mu.Unlock()
	if delay := time.Until(at); delay > 0 {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

// get fetches path (with query) from the API into out. A 429 answer is
// retried after a minute, up to three times.
func (d *discogsAPI) get(ctx context.Context, path string, out any) error {
	for attempt := 0; ; attempt++ {
		if err := d.wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", d.ua)
		req.Header.Set("Accept", "application/vnd.discogs.v2.discogs+json")
		if d.token != "" {
			req.Header.Set("Authorization", "Discogs token="+d.token)
		}
		resp, err := d.client.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return err
		}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt < 3:
			t := time.NewTimer(time.Minute)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
			continue
		case resp.StatusCode == http.StatusNotFound:
			return errNotFound
		case resp.StatusCode != http.StatusOK:
			return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("GET %s: %w", path, err)
		}
		return nil
	}
}

// API shapes (only the fields the site uses).

type apiArtist struct {
	Name string `json:"name"`
	ANV  string `json:"anv"`
	Join string `json:"join"`
	ID   int64  `json:"id"`
}

type apiFormat struct {
	Name         string   `json:"name"`
	Qty          string   `json:"qty"`
	Text         string   `json:"text"`
	Descriptions []string `json:"descriptions"`
}

type apiLabel struct {
	Name  string `json:"name"`
	Catno string `json:"catno"`
	ID    int64  `json:"id"`
}

type apiUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

type apiCollectionPage struct {
	Pagination struct {
		Page  int `json:"page"`
		Pages int `json:"pages"`
		Items int `json:"items"`
	} `json:"pagination"`
	Releases []struct {
		ID         int64 `json:"id"`
		InstanceID int64 `json:"instance_id"`
		Basic      struct {
			MasterID   int64       `json:"master_id"`
			CoverImage string      `json:"cover_image"`
			Title      string      `json:"title"`
			Year       int         `json:"year"`
			Formats    []apiFormat `json:"formats"`
			Labels     []apiLabel  `json:"labels"`
			Artists    []apiArtist `json:"artists"`
			Genres     []string    `json:"genres"`
			Styles     []string    `json:"styles"`
		} `json:"basic_information"`
		// Notes are the owner's own collection fields; only private ones
		// need the owner's token. Only the "Sort as" field is ever kept.
		Notes []struct {
			FieldID int    `json:"field_id"`
			Value   string `json:"value"`
		} `json:"notes"`
	} `json:"releases"`
}

// apiFields lists the owner's collection fields (names and ids).
type apiFields struct {
	Fields []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"fields"`
}

type apiTrack struct {
	Position  string      `json:"position"`
	Type      string      `json:"type_"`
	Title     string      `json:"title"`
	Duration  string      `json:"duration"`
	Artists   []apiArtist `json:"artists"`
	SubTracks []apiTrack  `json:"sub_tracks"`
}

type apiRelease struct {
	ID        int64      `json:"id"`
	Country   string     `json:"country"`
	Released  string     `json:"released"`
	Notes     string     `json:"notes"`
	Tracklist []apiTrack `json:"tracklist"`
	Images    []struct {
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"images"`
}

type apiMaster struct {
	Year int `json:"year"`
}

func (d *discogsAPI) user(ctx context.Context, name string) (apiUser, error) {
	var u apiUser
	err := d.get(ctx, "/users/"+url.PathEscape(name), &u)
	return u, err
}

// collectionFields lists the collection's own fields; without the owner's
// token Discogs returns only the public ones.
func (d *discogsAPI) collectionFields(ctx context.Context, name string) (apiFields, error) {
	var f apiFields
	err := d.get(ctx, "/users/"+url.PathEscape(name)+"/collection/fields", &f)
	return f, err
}

// collectionPage reads one page of folder 0 ("All"), newest first.
func (d *discogsAPI) collectionPage(ctx context.Context, name string, page int) (apiCollectionPage, error) {
	var p apiCollectionPage
	q := url.Values{"per_page": {"100"}, "page": {strconv.Itoa(page)}, "sort": {"added"}, "sort_order": {"desc"}}
	err := d.get(ctx, "/users/"+url.PathEscape(name)+"/collection/folders/0/releases?"+q.Encode(), &p)
	return p, err
}

func (d *discogsAPI) release(ctx context.Context, id int64) (apiRelease, error) {
	var r apiRelease
	err := d.get(ctx, "/releases/"+strconv.FormatInt(id, 10), &r)
	return r, err
}

func (d *discogsAPI) master(ctx context.Context, id int64) (apiMaster, error) {
	var m apiMaster
	err := d.get(ctx, "/masters/"+strconv.FormatInt(id, 10), &m)
	return m, err
}
