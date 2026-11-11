package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSortKey(t *testing.T) {
	cases := map[string]string{
		"The Beatles":        "beatles",
		"the cure":           "cure",
		"Théâtre":            "theatre",
		"a-ha":               "a-ha",
		"Давид Тухманов":     "давид тухманов",
		"Thereza":            "thereza",
		"  The Art Of Noise": "art of noise",
	}
	for in, want := range cases {
		if got := sortKey(in); got != want {
			t.Errorf("sortKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArtistsAndFormats(t *testing.T) {
	as := convertArtists([]apiArtist{{Name: "Wings (2)", ANV: "Paul McCartney & Wings", Join: "/"}, {Name: "Olivia Newton-John", Join: ","}, {Name: "Amina"}})
	if as[0].Name != "Paul McCartney & Wings" || as[0].Sort != "Wings" {
		t.Fatalf("ANV/sort: %+v", as[0])
	}
	if got := joinArtists(as); got != "Paul McCartney & Wings / Olivia Newton-John, Amina" {
		t.Errorf("joinArtists = %q", got)
	}
	r := Release{Formats: []Format{{Name: "Vinyl", Qty: "2", Text: "Orange", Descriptions: []string{"LP", "Album", "Reissue"}}}}
	if got := r.formatLines(); !reflect.DeepEqual(got, []string{"2 × Vinyl, LP, Album, Reissue, Orange"}) {
		t.Errorf("formatLines = %q", got)
	}
	if got := r.formatShort(); got != "2×LP" {
		t.Errorf("formatShort = %q", got)
	}
	r.Formats = []Format{{Name: "CD", Qty: "1", Descriptions: []string{"Album"}}}
	if got := r.formatShort(); got != "CD" {
		t.Errorf("formatShort CD = %q", got)
	}
}

func TestFormatReleased(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2023-09-15", "September 15, 2023"},
		{"1985-05-00", "May 1985"},
		{"1985-00-00", "1985"},
		{"1985", "1985"},
		{"", ""},
	}
	for _, c := range cases {
		if got := formatReleased(c.in); got != c.want {
			t.Errorf("formatReleased(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanNotes(t *testing.T) {
	in := "Recorded at [l=Abbey Road Studios].\r\nMixed by [a=Alan Parsons (2)][a12345].\r\n\r\n[b]Bold[/b] [url=https://x.example]link text[/url]"
	want := []string{"Recorded at Abbey Road Studios.\nMixed by Alan Parsons.", "Bold link text"}
	if got := cleanNotes(in); !reflect.DeepEqual(got, want) {
		t.Errorf("cleanNotes = %q, want %q", got, want)
	}
}

func TestReleasesCount(t *testing.T) {
	for n, want := range map[int]string{0: "0 releases", 1: "1 release", 2: "2 releases", 320: "320 releases"} {
		if got := releasesCount(n); got != want {
			t.Errorf("releasesCount(%d) = %q, want %q", n, got, want)
		}
	}
}

// fakeDiscogs serves a two-page collection of three records. primary is the
// file name of release 10's primary image; tests change it to mimic a new
// scan uploaded to Discogs.
func fakeDiscogs(t *testing.T) (*httptest.Server, *int, *string) {
	t.Helper()
	calls := new(int)
	primary := new(string)
	*primary = "10-primary"
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		if r.Header.Get("User-Agent") == "" {
			t.Errorf("request without User-Agent: %s", r.URL)
		}
		rel := func(id, inst, master int64, artist, title string, year int, added string) string {
			return fmt.Sprintf(`{"id":%d,"instance_id":%d,"date_added":%q,"rating":0,"basic_information":{"master_id":%d,
				"cover_image":"%s/img/%d.jpeg","title":%q,"year":%d,"formats":[{"name":"Vinyl","qty":"1","descriptions":["LP","Album"]}],
				"labels":[{"name":"EMI","catno":"PCS 1"},{"name":"EMI","catno":"PCS 1"}],"artists":[{"name":%q,"anv":"","join":"","id":1}],
				"genres":["Rock"],"styles":["Pop Rock"]}}`, id, inst, added, master, srv.URL, id, title, year, artist)
		}
		switch {
		case r.URL.Path == "/users/owner":
			io.WriteString(w, `{"username":"owner","name":"Record Owner","profile":"hi"}`)
		case r.URL.Path == "/users/owner/collection/folders/0/releases":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprintf(w, `{"pagination":{"page":1,"pages":2,"items":3},"releases":[%s,%s]}`,
					rel(10, 100, 1000, "The Beatles", "Abbey Road", 2019, "2025-01-02T10:00:00-08:00"),
					rel(11, 101, 1001, "The Beatles", "Please Please Me", 2012, "2024-01-02T10:00:00-08:00"))
			} else {
				fmt.Fprintf(w, `{"pagination":{"page":2,"pages":2,"items":3},"releases":[%s]}`,
					rel(12, 102, 0, "Bee Gees", "Main Course", 1975, "2023-01-02T10:00:00-08:00"))
			}
		case r.URL.Path == "/releases/10":
			fmt.Fprintf(w, `{"id":10,"country":"UK","released":"2019-09-27","notes":"Remix by [a=Giles Martin].",
				"images":[{"type":"secondary","uri":"%[1]s/img/10-back.jpeg"},{"type":"primary","uri":"%[1]s/img/%[2]s.jpeg"}],
				"tracklist":[{"position":"","type_":"heading","title":"Side One"},{"position":"A1","type_":"track","title":"Come Together","duration":"4:20"}]}`, srv.URL, *primary)
		case strings.HasPrefix(r.URL.Path, "/releases/"):
			http.NotFound(w, r)
		case r.URL.Path == "/masters/1000":
			io.WriteString(w, `{"year":1969}`)
		case r.URL.Path == "/masters/1001":
			io.WriteString(w, `{"year":1963}`)
		case strings.HasPrefix(r.URL.Path, "/img/"):
			// A JPEG signature followed by the file name, so files differ.
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, r.URL.Path...))
		default:
			t.Errorf("unexpected request %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, calls, primary
}

func testApp(t *testing.T) (*App, *int) {
	a, calls, _ := testAppPrimary(t)
	return a, calls
}

func testAppPrimary(t *testing.T) (*App, *int, *string) {
	t.Helper()
	srv, calls, primary := fakeDiscogs(t)
	a, err := newApp(Config{DataDir: t.TempDir(), DiscogsUser: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	a.discogs = newDiscogsAPI(srv.URL, "")
	a.discogs.gap = 0
	if err := a.syncOnce(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	return a, calls, primary
}

func get(t *testing.T, h http.Handler, path string, header ...string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestSyncAndPages(t *testing.T) {
	a, calls := testApp(t)
	var d Data
	a.store.view(func(x *Data) { d = *x })
	if len(d.Releases) != 3 || d.Owner.Name != "Record Owner" {
		t.Fatalf("store: %d releases, owner %+v", len(d.Releases), d.Owner)
	}
	if len(d.Releases[0].Labels) != 1 {
		t.Errorf("duplicate labels kept: %+v", d.Releases[0].Labels)
	}
	if d.MasterYears[1000] != 1969 || d.Details[10] == nil || d.Details[11] == nil {
		t.Errorf("details/masters: %+v %+v", d.MasterYears, d.Details)
	}
	if _, err := os.Stat(filepath.Join(a.coversDir, "10.jpg")); err != nil {
		t.Errorf("cover not stored: %v", err)
	}

	// A second sync reads only the list again: user + 2 pages.
	before := *calls
	if err := a.syncOnce(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if n := *calls - before; n != 3 {
		t.Errorf("second sync made %d requests, want 3", n)
	}

	h := a.routes()
	w := get(t, h, "/", "Accept-Language", "en")
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Record Owner") || !strings.Contains(body, `"t":"Abbey Road"`) {
		t.Fatalf("index: %d\n%s", w.Code, body)
	}
	// Artist order ignores "The"; the same artist goes by original year.
	pp, ar, mc := strings.Index(body, `"t":"Please Please Me"`), strings.Index(body, `"t":"Abbey Road"`), strings.Index(body, `"t":"Main Course"`)
	if !(pp < ar && ar < mc) {
		t.Errorf("order: Please Please Me %d, Abbey Road %d, Main Course %d", pp, ar, mc)
	}
	if !strings.Contains(body, `id="order"`) || !strings.Contains(body, `"order_desc":"Reverse order"`) {
		t.Error("index lacks the order toggle or its texts")
	}
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("CSP: %q", csp)
	}

	w = get(t, h, "/release/100/the-beatles-abbey-road", "Accept-Language", "ru")
	body = w.Body.String()
	for _, want := range []string{"Come Together", "Side One", "1969", "September 27, 2019", "Remix by Giles Martin.", "https://www.discogs.com/release/10", "<html"} {
		if !strings.Contains(body, want) {
			t.Errorf("release page lacks %q", want)
		}
	}
	w = get(t, h, "/release/100?part=1")
	if b := w.Body.String(); strings.Contains(b, "<html") || !strings.Contains(b, "<article") {
		t.Errorf("part=1 should be the article only:\n%s", b)
	}
	// Nothing personal: no date added on the card (the fake adds records in
	// 2023–2025; the release itself is from 1969/2019).
	b := get(t, h, "/release/100?part=1").Body.String()
	for _, y := range []string{"2023", "2024", "2025"} {
		if strings.Contains(b, y) {
			t.Errorf("card contains %s (date added?)", y)
		}
	}
	if b := get(t, h, "/").Body.String(); strings.Contains(b, `"d":`) {
		t.Errorf("page data carries the date added")
	}
	if w := get(t, h, "/release/999"); w.Code != 404 {
		t.Errorf("unknown release: %d", w.Code)
	}
	if w := get(t, h, "/cover/10.jpg"); w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("cover: %d %s", w.Code, w.Header().Get("Content-Type"))
	}
	for _, bad := range []string{"/cover/..%2Fcatalogue.json", "/cover/10.png", "/cover/x.jpg"} {
		if w := get(t, h, bad); w.Code != 404 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
}

// The site is English only: no language switch, whatever the browser asks.
func TestEnglishOnly(t *testing.T) {
	a, _ := testApp(t)
	h := a.routes()
	if w := get(t, h, "/lang/ru?back=/"); w.Code != 404 {
		t.Errorf("/lang/ru: %d, want 404", w.Code)
	}
	for _, path := range []string{"/", "/release/100/the-beatles-abbey-road"} {
		w := get(t, h, path, "Accept-Language", "ru")
		b := w.Body.String()
		if !strings.Contains(b, `<html lang="en">`) || strings.Contains(b, "/lang/") || strings.ContainsAny(b, "абвгдеёжзийклмнопрстуфхцчшщыэюя") {
			t.Errorf("%s is not English only", path)
		}
		if c := w.Header().Get("Set-Cookie"); c != "" {
			t.Errorf("%s sets a cookie: %s", path, c)
		}
	}
}

func TestStoreSurvivesRestart(t *testing.T) {
	a, _ := testApp(t)
	st, err := openStore(a.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	var tracks int
	st.view(func(d *Data) { n = len(d.Releases); tracks = len(d.Details[10].Tracklist) })
	if n != 3 || tracks != 2 {
		t.Errorf("reloaded store: %d releases, %d tracks", n, tracks)
	}
	b, _ := os.ReadFile(filepath.Join(a.cfg.DataDir, "catalogue.json"))
	if !json.Valid(b) {
		t.Error("catalogue.json is not valid JSON")
	}
}

func TestOriginalYear(t *testing.T) {
	d := &Data{MasterYears: map[int64]int{7: 1985}}
	cases := []struct {
		r          Release
		orig, sort int
	}{
		{Release{MasterID: 7, Year: 2023}, 1985, 1985},
		{Release{MasterID: 8, Year: 2023}, 0, 2023}, // master not read yet
		{Release{MasterID: 0, Year: 1977}, 1977, 1977},
	}
	for _, c := range cases {
		if o, s := originalYear(d, &c.r), sortYear(d, &c.r); o != c.orig || s != c.sort {
			t.Errorf("%+v: originalYear %d sortYear %d, want %d %d", c.r, o, s, c.orig, c.sort)
		}
	}
}

func TestFullSyncReplacesCover(t *testing.T) {
	a, calls, primary := testAppPrimary(t)
	cover := filepath.Join(a.coversDir, "10.jpg")
	b, _ := os.ReadFile(cover)
	if !strings.Contains(string(b), "10-primary") {
		t.Fatalf("cover should come from the primary image, got %q", b)
	}
	var url1 string
	a.store.view(func(d *Data) { url1 = a.coverURL(d, &d.Releases[0]) })

	// Someone uploads a better scan on Discogs.
	*primary = "10-better"
	before := *calls
	if err := a.syncOnce(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	// user + 2 pages + 3 releases + the new image; masters are known already.
	if n := *calls - before; n != 1+2+3+1 {
		t.Errorf("full sync made %d requests", n)
	}
	b, _ = os.ReadFile(cover)
	if !strings.Contains(string(b), "10-better") {
		t.Errorf("cover not replaced: %q", b)
	}
	var url2 string
	var full time.Time
	a.store.view(func(d *Data) { url2 = a.coverURL(d, &d.Releases[0]); full = d.FullSyncAt })
	if url1 == url2 || url2 == "" {
		t.Errorf("cover URL should change with the image: %q → %q", url1, url2)
	}
	if time.Since(full) > time.Minute {
		t.Errorf("FullSyncAt not set: %v", full)
	}
}

func TestLinkPreviews(t *testing.T) {
	a, _ := testApp(t)
	h := a.routes()
	// The collection page: the English card whatever the visitor's language.
	b := get(t, h, "/", "Accept-Language", "ru").Body.String()
	for _, want := range []string{
		`<meta property="og:title" content="Record Owner&#39;s record collection">`,
		`<meta property="og:description" content="3 releases. Dig through the crates.">`,
		`<meta property="og:image" content="http://example.com/static/og.png?v=dev">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta property="og:url" content="http://example.com/">`,
		`<meta property="og:locale" content="en_US">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`href="/static/favicon-32.png?v=dev"`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("index lacks %s", want)
		}
	}
	// A record's page: its own title, address and English description. The
	// test covers are not real JPEGs, so the card stays as the image.
	b = get(t, h, "/release/100/the-beatles-abbey-road", "Accept-Language", "ru").Body.String()
	for _, want := range []string{
		`<meta property="og:title" content="The Beatles – Abbey Road">`,
		`<meta property="og:url" content="http://example.com/release/100/the-beatles-abbey-road">`,
		`<meta property="og:description" content="1969 · 1 × Vinyl, LP, Album · From Record Owner&#39;s record collection">`,
	} {
		if !strings.Contains(b, want) {
			t.Errorf("release page lacks %s", want)
		}
	}
	for path, ctype := range map[string]string{"/static/og.png": "image/png", "/static/favicon-32.png": "image/png"} {
		if w := get(t, h, path); w.Code != 200 || w.Header().Get("Content-Type") != ctype {
			t.Errorf("%s: %d %s", path, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

func TestCoverSize(t *testing.T) {
	a, _ := testApp(t)
	// A real 3×2 JPEG in place of a test cover.
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	f, err := os.Create(a.coverPath(10))
	if err != nil {
		t.Fatal(err)
	}
	jpeg.Encode(f, img, nil)
	f.Close()
	if w, h, ok := a.coverSize(10); !ok || w != 3 || h != 2 {
		t.Errorf("coverSize = %d×%d %v", w, h, ok)
	}
	b := get(t, a.routes(), "/release/100").Body.String()
	for _, want := range []string{`og:image:type" content="image/jpeg"`, `og:image:width" content="3"`, `twitter:card" content="summary"`, `/cover/10.jpg?v=`} {
		if !strings.Contains(b, want) {
			t.Errorf("release with a real cover lacks %s", want)
		}
	}
}

// The debug log route exists only on test copies (DEBUG_LOG=1).
func TestDebugLogRoute(t *testing.T) {
	a, _ := testApp(t)
	t.Setenv("DEBUG_LOG", "")
	if w := post(t, a.routes(), "/debug/wheel-log", `{"x":1}`); w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("without DEBUG_LOG: %d, want 404/405", w.Code)
	}
	t.Setenv("DEBUG_LOG", "1")
	if w := post(t, a.routes(), "/debug/wheel-log", `{"x":1}`); w.Code != http.StatusNoContent {
		t.Errorf("with DEBUG_LOG: %d, want 204", w.Code)
	}
	b, err := os.ReadFile(filepath.Join(a.cfg.DataDir, "debug-wheel-log.json"))
	if err != nil || string(b) != `{"x":1}` {
		t.Errorf("log file: %q %v", b, err)
	}
}

func post(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(body)))
	return w
}
