package main

import (
	"context"
	"encoding/json"
	"fmt"
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
	cases := []struct{ lang, in, want string }{
		{"en", "2023-09-15", "September 15, 2023"},
		{"ru", "2023-09-15", "15 сентября 2023"},
		{"en", "1985-05-00", "May 1985"},
		{"ru", "1985-05-00", "Май 1985"},
		{"en", "1985-00-00", "1985"},
		{"en", "1985", "1985"},
		{"en", "", ""},
	}
	for _, c := range cases {
		if got := formatReleased(c.lang, c.in); got != c.want {
			t.Errorf("formatReleased(%s, %q) = %q, want %q", c.lang, c.in, got, c.want)
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

func TestPluralRU(t *testing.T) {
	for n, want := range map[int]string{1: "1 релиз", 2: "2 релиза", 5: "5 релизов", 11: "11 релизов", 21: "21 релиз", 112: "112 релизов", 320: "320 релизов"} {
		if got := releasesCount("ru", n); got != want {
			t.Errorf("releasesCount(ru, %d) = %q, want %q", n, got, want)
		}
	}
}

func TestDetectLang(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "uk-UA,ru;q=0.8,en;q=0.5")
	if got := detectLang(r); got != "ru" {
		t.Errorf("Accept-Language: got %s", got)
	}
	r.AddCookie(&http.Cookie{Name: "lang", Value: "en"})
	if got := detectLang(r); got != "en" {
		t.Errorf("cookie: got %s", got)
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
	for _, want := range []string{"Come Together", "Side One", "1969", "27 сентября 2019", "Remix by Giles Martin.", "https://www.discogs.com/release/10", "<html"} {
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
	for _, lang := range []string{"en", "ru"} {
		b := get(t, h, "/release/100?part=1", "Accept-Language", lang).Body.String()
		for _, y := range []string{"2023", "2024", "2025"} {
			if strings.Contains(b, y) {
				t.Errorf("%s card contains %s (date added?)", lang, y)
			}
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

func TestLangSwitch(t *testing.T) {
	a, _ := testApp(t)
	h := a.routes()
	w := get(t, h, "/lang/ru?back=/release/100")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/release/100" || !strings.Contains(w.Header().Get("Set-Cookie"), "lang=ru") {
		t.Errorf("switch: %d %v", w.Code, w.Header())
	}
	for _, back := range []string{"//evil.example", "https://evil.example", "/\\evil"} {
		w := get(t, h, "/lang/en?back="+back)
		if loc := w.Header().Get("Location"); loc != "/" {
			t.Errorf("back=%s redirected to %q", back, loc)
		}
	}
	if w := get(t, h, "/lang/de"); w.Code != 404 {
		t.Errorf("unknown language: %d", w.Code)
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
