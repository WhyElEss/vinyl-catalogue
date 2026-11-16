package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSimRatioMatchesDifflib(t *testing.T) {
	// reference values from Python's difflib.SequenceMatcher(None, a, b).ratio()
	for _, c := range []struct {
		a, b string
		want float64
	}{
		{"saudade part two", "saudade", 0.6086956521739131},
		{"miami vice ii", "miami vice", 0.8695652173913043},
		{"history of", "best of 20th century masters millennium collection", 0.23333333333333334},
		{"abcab", "bcabc", 0.8},
		{"музыка во вселенной", "music in the universe", 0.1},
		{"long play ii", "longplay album volume ii", 0.6111111111111112},
	} {
		if got := simRatio(c.a, c.b); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("simRatio(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestAppleTextRules(t *testing.T) {
	if got := anorm("The Beatles — Sgt. Pepper's & Friends™"); got != "beatles sgt pepper s and friends" {
		t.Errorf("anorm = %q", got)
	}
	if got := acore("Low End Love Songs (Instrumentals) - EP"); got != "low end love songs" {
		t.Errorf("acore = %q", got)
	}
	if got := anumbers("Guardians Of The Galaxy Vol3 (1983)"); !sameSet(got, map[string]bool{"3": true}) {
		t.Errorf("anumbers = %v", got)
	}
	if !hasWord("david bowie", "bowie") || hasWord("bowiesque", "bowie") || !hasWord("тухманов давид", "давид") {
		t.Error("hasWord")
	}
	if got := discogsSeconds([]Track{{Position: "", Heading: true, Title: "Side A"}, {Position: "A1", Duration: "4:50"},
		{Position: "A2", Duration: "1:02:03"}, {Position: "A3"}}); fmt.Sprint(got) != "[290 3723]" {
		t.Errorf("discogsSeconds = %v", got)
	}
	if got := durationRatio([]int{285, 251, 212, 283}, []int{290, 256, 216, 400}); got != 0.75 {
		t.Errorf("durationRatio = %v", got)
	}
	if scriptOf("魔女の宅急便") != "other" || scriptOf("Kiki's Delivery Service") != "latin" {
		t.Error("scriptOf")
	}
	r := &Release{Title: "Low End Love Songs", Artists: []Artist{{Name: "Pale Jay", ID: 1}}}
	if got := spotifySearchURL(r); got != "https://open.spotify.com/search/Pale%20Jay%20Low%20End%20Love%20Songs/albums" {
		t.Errorf("spotify = %s", got)
	}
	r = &Release{Title: "Tower / Power", Artists: []Artist{{Name: "Various", ID: 194}}}
	if got := spotifySearchURL(r); got != "https://open.spotify.com/search/Tower%20%2F%20Power/albums" {
		t.Errorf("spotify compilation = %s", got)
	}
}

// fakeApple answers like the iTunes Search API from a few fixed albums.
func fakeApple(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	album := func(id, artistID int64, artist, title, date string, tracks int) string {
		return fmt.Sprintf(`{"wrapperType":"collection","collectionId":%d,"artistId":%d,"artistName":%q,"collectionName":%q,
			"releaseDate":%q,"trackCount":%d,"collectionViewUrl":"https://music.apple.com/ca/album/x/%d?uo=4"}`, id, artistID, artist, title, date, tracks, id)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		*calls = append(*calls, r.URL.Path+"?"+q.Encode())
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("request without User-Agent: %s", r.URL)
		}
		res := []string{}
		switch {
		case q.Get("upc") == "0123456789012":
			res = append(res, album(500, 50, "The Beatles", "Abbey Road (Remastered)", "1969-09-26T07:00:00Z", 17))
		case q.Get("entity") == "musicArtist" && q.Get("term") == "Bee Gees":
			res = append(res, `{"wrapperType":"artist","artistId":60,"artistName":"Bee Gees"}`)
		case q.Get("id") == "60" && q.Get("entity") == "album":
			res = append(res, `{"wrapperType":"artist","artistId":60,"artistName":"Bee Gees"}`,
				album(601, 60, "Bee Gees", "Main Course", "1975-06-01T07:00:00Z", 10),
				album(602, 60, "Bee Gees", "Main Course (Live)", "1990-06-01T07:00:00Z", 10))
		case q.Get("entity") == "song":
			for _, id := range strings.Split(q.Get("id"), ",") {
				if id == "601" {
					for _, ms := range []int{290000, 256000, 216000} {
						res = append(res, fmt.Sprintf(`{"wrapperType":"track","collectionId":601,"trackTimeMillis":%d}`, ms))
					}
				}
			}
		}
		fmt.Fprintf(w, `{"resultCount":%d,"results":[%s]}`, len(res), strings.Join(res, ","))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMatchApple(t *testing.T) {
	a, _ := testApp(t)
	var calls []string
	a.apple = newAppleAPI(fakeApple(t, &calls).URL)
	a.apple.gap = 0
	// barcodes and durations as the details would bring them
	a.store.update(func(d *Data) {
		d.Details[10].Barcodes = []string{"0123456789012"}
		d.Details[12] = &Details{IDsRead: true, Tracklist: []Track{{Position: "A1", Duration: "4:50"}, {Position: "A2", Duration: "4:16"}, {Position: "A3", Duration: "3:36"}}}
	})
	appleManual[11] = 777 // as for Kiki: given by hand
	defer delete(appleManual, 11)
	a.store.update(func(d *Data) { d.Apple[11] = &AppleMatch{Album: 1, Status: "none"} })

	var releases []Release
	a.store.view(func(d *Data) { releases = append(releases, d.Releases...) })
	if err := a.matchApple(context.Background(), releases); err != nil {
		t.Fatal(err)
	}
	var got map[int64]AppleMatch
	a.store.view(func(d *Data) {
		got = map[int64]AppleMatch{}
		for k, v := range d.Apple {
			got[k] = *v
		}
	})
	if m := got[10]; m.Status != "barcode" || m.Album != 500 {
		t.Errorf("Abbey Road = %+v, want barcode 500", m)
	}
	if m := got[11]; m.Status != "manual" || m.Album != 777 {
		t.Errorf("manual album = %+v, want manual 777", m)
	}
	if m := got[12]; m.Status != "found" || m.Album != 601 {
		t.Errorf("Main Course = %+v, want found 601 (not the live album)", m)
	}

	// The card links the album; a full refresh neither searches again nor touches the manual album.
	body := get(t, a.routes(), "/release/102/x?part=1").Body.String()
	if !strings.Contains(body, `href="https://geo.music.apple.com/album/601"`) {
		t.Errorf("card has no Apple Music link:\n%s", body)
	}
	if !strings.Contains(body, `href="https://open.spotify.com/search/Bee%20Gees%20Main%20Course/albums"`) {
		t.Errorf("card has no Spotify link:\n%s", body)
	}
	if n := strings.Count(body, `<svg class="icon"`); n != 3 {
		t.Errorf("card has %d link icons, want 3 (Discogs, Apple Music, Spotify)", n)
	}
	n := len(calls)
	if err := a.matchApple(context.Background(), releases); err != nil {
		t.Fatal(err)
	}
	if len(calls) != n {
		t.Errorf("second run made %d requests, want none: %v", len(calls)-n, calls[n:])
	}
	a.store.view(func(d *Data) {
		if m := d.Apple[11]; m.Status != "manual" || m.Album != 777 {
			t.Errorf("manual album after second run = %+v", m)
		}
	})
	// Without a link: no Apple Music link on the card, Spotify still there.
	a.store.update(func(d *Data) { d.Apple[12] = &AppleMatch{Album: 602, Status: "unsure", CheckedAt: time.Now()} })
	body = get(t, a.routes(), "/release/102/x?part=1").Body.String()
	if strings.Contains(body, "geo.music.apple.com") || !strings.Contains(body, "open.spotify.com") {
		t.Errorf("unsure match must show no Apple Music link:\n%s", body)
	}
}

// TestAppleAgainstReview replays the Apple answers recorded during the owner's
// review (2026-11-16) and checks that this code picks the same album with the
// same status as the review scripts for every release. It runs only with
// VINYL_APPLE_FIXTURES set to a folder with catalogue.json, http_cache.json,
// apple.json (barcode hits) and apple3.json (the reviewed results).
func TestAppleAgainstReview(t *testing.T) {
	dir := os.Getenv("VINYL_APPLE_FIXTURES")
	if dir == "" {
		t.Skip("VINYL_APPLE_FIXTURES not set")
	}
	read := func(name string, v any) {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, v); err != nil {
			t.Fatal(err)
		}
	}
	var data Data
	read("catalogue.json", &data)
	var cache map[string]struct {
		Results []json.RawMessage `json:"results"`
	}
	read("http_cache.json", &cache)
	var v1 map[string]struct {
		UpcHit *struct {
			CollectionID int64  `json:"collectionId"`
			Via          string `json:"via"`
		} `json:"upc_hit"`
	}
	read("apple.json", &v1)
	var want map[string]struct {
		St   string `json:"st"`
		Best *struct {
			CollectionID int64 `json:"collectionId"`
		} `json:"best"`
	}
	read("apple3.json", &want)
	var barcodes map[string][]string
	read("barcodes.json", &barcodes)

	key := func(path string, q url.Values) string {
		var ks []string
		for k, vs := range q {
			ks = append(ks, k+"="+vs[0])
		}
		sort.Strings(ks)
		return path + "?" + strings.Join(ks, "&")
	}
	answers := map[string][]json.RawMessage{}
	songs := map[int64][]json.RawMessage{}
	items := map[int64]json.RawMessage{}
	for u, j := range cache {
		pu, _ := url.Parse(u)
		answers[key(pu.Path, pu.Query())] = j.Results
		for _, raw := range j.Results {
			var x appleItem
			json.Unmarshal(raw, &x)
			if x.WrapperType == "track" {
				songs[x.CollectionID] = append(songs[x.CollectionID], raw)
			} else if x.WrapperType == "collection" {
				items[x.CollectionID] = raw
			}
		}
	}
	// v1 asked for barcodes without the cache; its hits are in apple.json
	upc := map[string]json.RawMessage{}
	for _, r := range v1 {
		if r.UpcHit != nil {
			upc[strings.TrimPrefix(r.UpcHit.Via, "barcode ")] = items[r.UpcHit.CollectionID]
		}
	}
	missing := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var res []json.RawMessage
		switch {
		case q.Get("upc") != "":
			if x, ok := upc[q.Get("upc")]; ok && x != nil {
				res = append(res, x)
			}
		case q.Get("entity") == "song":
			seen := map[int64]bool{}
			for _, id := range strings.Split(q.Get("id"), ",") {
				n, _ := strconv.ParseInt(id, 10, 64)
				if !seen[n] {
					seen[n] = true
					res = append(res, songs[n]...)
				}
			}
		default:
			var ok bool
			res, ok = answers[key("/"+strings.TrimPrefix(r.URL.Path, "/"), q)]
			if !ok {
				missing++
				t.Logf("not recorded: %s", r.URL)
			}
		}
		b, _ := json.Marshal(map[string]any{"results": res})
		w.Write(b)
	}))
	defer srv.Close()
	api := newAppleAPI(srv.URL)
	api.gap = 0
	m := newAppleMatcher(api)

	same, diff := 0, 0
	done := map[int64]bool{}
	for i := range data.Releases {
		r := &data.Releases[i]
		if done[r.ID] {
			continue
		}
		done[r.ID] = true
		det := data.Details[r.ID]
		if det == nil {
			det = &Details{}
		}
		det.Barcodes = barcodes[strconv.FormatInt(r.ID, 10)]
		got, err := m.match(context.Background(), appleRelFor(&data, r, det))
		if err != nil {
			t.Fatal(err)
		}
		w := want[strconv.FormatInt(r.ID, 10)]
		var wantID int64
		if w.Best != nil {
			wantID = w.Best.CollectionID
		}
		if got.Status == w.St && (got.Album == wantID || (got.Status == "none" && w.St == "none")) {
			same++
			continue
		}
		diff++
		t.Errorf("%d %s – %s: got %s %d (%.3f), review %s %d", r.ID, r.artistLine(), r.Title, got.Status, got.Album, got.Score, w.St, wantID)
	}
	t.Logf("%d releases as in the review, %d different, %d requests not recorded", same, diff, missing)
}
