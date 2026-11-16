package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Apple Music links for the release card.
//
// The album is found with Apple's free iTunes Search API
// (https://performance-partners.apple.com/search-api), which needs no key and
// allows about 20 requests a minute. Following its advice, ID lookups come
// first and text searches are narrowed with `attribute`:
//
//  1. the release's barcodes (Discogs identifiers): lookup?upc=, Canada then US;
//  2. every credited artist: search entity=musicArtist&attribute=artistTerm,
//     then all its albums: lookup?id=<artist>&entity=album (the first two
//     artists of that name; up to five when nothing sure was found, since
//     several artists share names such as "Zodiac");
//  3. only when still nothing sure: search entity=album&attribute=albumTerm
//     by the title, Canada, then the US store for artists and titles;
//  4. track durations (lookup?id=a,b,c&entity=song) against the Discogs
//     tracklist confirm an album whose title cannot be compared (another
//     alphabet) and are required when Apple files the album under someone
//     who is not credited on the record.
//
// The rules and thresholds were set on the owner's collection of 319 releases
// and checked against three rounds of the owner's own review (2026-11-16):
// no rejected answer gets a link. Only "barcode", "found" and "manual" get a
// link on the card; "unsure" and "none" get none.

// appleManual are albums the owner gave by hand, keyed by Discogs release id.
// The search never changes them.
var appleManual = map[int64]int64{
	14905875: 882658777, // 久石譲 – 魔女の宅急便 サントラ音楽集: Japanese title, no durations on Discogs, a Japanese barcode Apple does not know
}

// AppleMatch is the Apple Music album found for one Discogs release.
type AppleMatch struct {
	Album     int64     `json:"album,omitempty"`
	Status    string    `json:"status"` // manual, barcode, found, unsure, none
	Score     float64   `json:"score,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// linked says whether the card shows the Apple Music link.
func (m *AppleMatch) linked() bool {
	return m != nil && m.Album != 0 && (m.Status == "manual" || m.Status == "barcode" || m.Status == "found")
}

// appleRecheck is how long a release without a link waits for another try:
// albums get added to Apple Music over time.
const appleRecheck = 30 * 24 * time.Hour

func appleAlbumURL(id int64) string {
	// geo.music.apple.com sends each visitor to their own country's store
	// (measured from Canada: /ca/); music.apple.com/album/<id> always went to /us/.
	return "https://geo.music.apple.com/album/" + strconv.FormatInt(id, 10)
}

func spotifySearchURL(r *Release) string {
	q := r.Title
	if !r.compilation() && len(r.Artists) > 0 {
		q = r.Artists[0].Name + " " + r.Title
	}
	q = strings.Join(strings.Fields(q), " ")
	return "https://open.spotify.com/search/" + url.PathEscape(q) + "/albums"
}

// ---------- API client ----------

type appleAPI struct {
	base   string // https://itunes.apple.com, a test server in tests
	ua     string
	client *http.Client

	mu   sync.Mutex
	gap  time.Duration
	next time.Time
}

func newAppleAPI(base string) *appleAPI {
	return &appleAPI{base: base, ua: "VinylCatalogue/" + version, client: &http.Client{Timeout: 30 * time.Second},
		gap: 3100 * time.Millisecond} // "approximately 20 calls per minute"
}

func (p *appleAPI) wait(ctx context.Context) error {
	p.mu.Lock()
	now := time.Now()
	at := p.next
	if at.Before(now) {
		at = now
	}
	p.next = at.Add(p.gap)
	p.mu.Unlock()
	if d := time.Until(at); d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	return nil
}

type appleItem struct {
	WrapperType     string `json:"wrapperType"`
	ArtistID        int64  `json:"artistId"`
	ArtistName      string `json:"artistName"`
	CollectionID    int64  `json:"collectionId"`
	CollectionName  string `json:"collectionName"`
	CollectionURL   string `json:"collectionViewUrl"`
	ReleaseDate     string `json:"releaseDate"`
	TrackCount      int    `json:"trackCount"`
	TrackTimeMillis int64  `json:"trackTimeMillis"`
}

// get asks /search or /lookup. 403 and 429 (Apple's answers to too many
// requests) are retried after a minute, up to three times.
func (p *appleAPI) get(ctx context.Context, path string, q url.Values) ([]appleItem, error) {
	for attempt := 0; ; attempt++ {
		if err := p.wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+path+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", p.ua)
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden) && attempt < 3 {
			t := time.NewTimer(time.Minute)
			select {
			case <-ctx.Done():
				t.Stop()
				return nil, ctx.Err()
			case <-t.C:
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
		}
		var out struct {
			Results []appleItem `json:"results"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("GET %s: %w", path, err)
		}
		return out.Results, nil
	}
}

// ---------- text helpers (the same rules as the review scripts) ----------

var (
	appleJunk = setOf("remaster remastered remasters deluxe edition expanded anniversary version bonus track tracks mono stereo " +
		"soundtrack ost original motion picture music from the of score reissue digital special collector collectors " +
		"th st nd rd and series film movie album lp volume vol pt")
	appleBad = setOf("live instrumental instrumentals remix remixes karaoke acoustic demo demos cover covers tribute " +
		"rehearsal sessions reprise unplugged a cappella")
	appleRoman = map[string]string{"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8",
		"ix": "9", "x": "10", "one": "1", "two": "2", "three": "3", "four": "4", "five": "5"}
	reBrackets = regexp.MustCompile(`\s*[(\[][^)\]]*[)\]]`)
	reSingleEP = regexp.MustCompile(`(?i)\s+-\s+(single|ep)\s*$`)
	reLeadThe  = regexp.MustCompile(`^the\s+`)
	// what Python's NFKD did in the review scripts: й -> и, ₂ -> 2 (Hall & Oates' "H₂O")
	foldAppleOK = strings.NewReplacer("ё", "е", "й", "и", "&", " and ", "™", "", "®", "",
		"₀", "0", "₁", "1", "₂", "2", "₃", "3", "₄", "4", "₅", "5", "₆", "6", "₇", "7", "₈", "8", "₉", "9",
		"⁰", "0", "¹", "1", "²", "2", "³", "3", "⁴", "4", "⁵", "5", "⁶", "6", "⁷", "7", "⁸", "8", "⁹", "9")
)

func setOf(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

// anorm lowercases, folds accents, keeps letters and digits, drops a leading "the".
func anorm(s string) string {
	s = foldAccents.Replace(foldAppleOK.Replace(strings.ToLower(s)))
	var b strings.Builder
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if isWordRune(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	s = reLeadThe.ReplaceAllString(strings.TrimSpace(b.String()), "")
	return strings.Join(strings.Fields(s), " ")
}

func acore(t string) string {
	t = reBrackets.ReplaceAllString(t, " ")
	t = reSingleEP.ReplaceAllString(t, "")
	return anorm(t)
}

func awords(t string) []string { return strings.Fields(anorm(t)) }

// anumbers are the numbers in a title: digits (not years), Roman numerals and one…five.
func anumbers(t string) map[string]bool {
	var b strings.Builder
	prev := rune(0)
	for _, r := range anorm(t) {
		if prev >= 'a' && prev <= 'z' && unicode.IsDigit(r) {
			b.WriteByte(' ') // "vol3" -> "vol 3"
		}
		b.WriteRune(r)
		prev = r
	}
	out := map[string]bool{}
	for _, w := range strings.Fields(b.String()) {
		if allDigits(w) {
			if len(w) == 4 && (w[:2] == "19" || w[:2] == "20") {
				continue
			}
			n, _ := strconv.Atoi(w)
			out[strconv.Itoa(n)] = true
		} else if v, ok := appleRoman[w]; ok {
			out[v] = true
		}
	}
	return out
}

func allDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return s != ""
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// simRatio is Python's difflib.SequenceMatcher(None, a, b).ratio() on runes.
func simRatio(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra)+len(rb) == 0 {
		return 1
	}
	return 2 * float64(matchingRunes(ra, rb)) / float64(len(ra)+len(rb))
}

func matchingRunes(a, b []rune) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// longest common block, earliest in a, then earliest in b
	bi, bj, bk := 0, 0, 0
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
				if k := cur[j]; k > bk || (k == bk && (i-k < bi || (i-k == bi && j-k < bj))) {
					bi, bj, bk = i-k, j-k, k
				}
			} else {
				cur[j] = 0
			}
		}
		prev, cur = cur, prev
	}
	if bk == 0 {
		return 0
	}
	return bk + matchingRunes(a[:bi], b[:bj]) + matchingRunes(a[bi+bk:], b[bj+bk:])
}

func asim(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	return simRatio(a, b)
}

// hasWord says whether needle occurs in hay as whole words.
func hasWord(hay, needle string) bool {
	if needle == "" {
		return false
	}
	for i := 0; ; {
		k := strings.Index(hay[i:], needle)
		if k < 0 {
			return false
		}
		s, e := i+k, i+k+len(needle)
		before := s == 0 || !isWordRune(lastRune(hay[:s]))
		after := e == len(hay) || !isWordRune(firstRune(hay[e:]))
		if before && after {
			return true
		}
		i = s + 1
		for i < len(hay) && !utf8Start(hay[i]) {
			i++
		}
	}
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }
func lastRune(s string) rune {
	r := []rune(s)
	return r[len(r)-1]
}
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func titleVariants(t string) []string {
	vs := []string{t}
	for _, sep := range []string{"=", " / "} {
		if strings.Contains(t, sep) {
			for _, p := range strings.Split(t, sep) {
				if p = strings.TrimSpace(p); p != "" {
					vs = append(vs, p)
				}
			}
		}
	}
	return vs
}

func stripArtist(t string, names []string) string {
	nt := anorm(t)
	for _, n := range names {
		nn := anorm(n)
		if nn != "" && strings.HasPrefix(nt, nn+" ") && len([]rune(nt)) > len([]rune(nn))+2 {
			return strings.TrimSpace(nt[len(nn):])
		}
	}
	return t
}

func joinNotIn(ws []string, skip map[string]bool, sep string) string {
	var out []string
	for _, w := range ws {
		if !skip[w] {
			out = append(out, w)
		}
	}
	return strings.Join(out, sep)
}

func wordSet(ws []string) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w] = true
	}
	return m
}

func anyIn(a, b map[string]bool, only map[string]bool) bool {
	for w := range a {
		if !b[w] && only[w] {
			return true
		}
	}
	return false
}

func isVariousName(n string) bool { return n == "various artists" || n == "various" }

// titleScore compares the Discogs title with an Apple album title.
func titleScore(dt, at string, names []string, appleVarious bool) float64 {
	aVariants := []string{at, stripArtist(at, names)}
	if strings.Contains(at, "/") {
		for _, p := range strings.Split(at, "/") {
			if p = strings.TrimSpace(p); p != "" {
				aVariants = append(aVariants, p)
			}
		}
	}
	if i := strings.Index(at, ":"); i >= 0 && appleVarious {
		// "Давид Тухманов: По волне моей памяти": only when the part before the colon names the artist
		head := anorm(at[:i])
		for _, n := range names[:len(names)-1] {
			if nn := anorm(n); nn != "" && !isVariousName(nn) && strings.Contains(head, nn) {
				aVariants = append(aVariants, at[i+1:])
				break
			}
		}
	}
	nameWords := map[string]bool{}
	for _, n := range names {
		for _, w := range awords(n) {
			nameWords[w] = true
		}
	}
	delete(nameWords, "various")
	delete(nameWords, "artists")
	deartist := func(t string) string {
		w := joinNotIn(strings.Fields(t), nameWords, " ")
		if w == "" {
			return t
		}
		return w
	}
	best := 0.0
	for _, d0 := range append(titleVariants(dt), stripArtist(dt, names)) {
		for _, a0 := range aVariants {
			cd, ca := acore(d0), acore(a0)
			if anyIn(wordSet(strings.Fields(cd)), nil, nameWords) && anyIn(wordSet(strings.Fields(ca)), nil, nameWords) {
				// "History Of The Righteous Brothers" vs "The Best of the Righteous Brothers": compare without the name
				cd, ca = deartist(cd), deartist(ca)
			}
			c := asim(cd, ca)
			dw, aw := awords(d0), awords(a0)
			full := asim(joinNotIn(dw, appleJunk, " "), joinNotIn(aw, appleJunk, " "))
			s := 0.75*c + 0.25*full
			if sq := joinNotIn(dw, appleJunk, ""); sq != "" && sq == joinNotIn(aw, appleJunk, "") {
				s = math.Max(s, 0.95) // "Long Play II" == "Longplay Album Volume II"
			}
			ds, as := wordSet(dw), wordSet(aw)
			if anyIn(ds, as, appleBad) {
				s -= 0.3
			} else if anyIn(as, ds, appleBad) {
				s -= 0.15 // Apple names a live album "(Live)" where Discogs may not
			}
			nd, na := anumbers(d0), anumbers(a0)
			if len(nd) > 0 && len(na) > 0 && !sameSet(nd, na) {
				s -= 0.25 // "II" vs "I", "90" vs "16"
			} else if !sameSet(nd, na) {
				s -= 0.1 // "Part Two" vs nothing
			}
			best = math.Max(best, s)
		}
	}
	return best
}

func round(x float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.RoundToEven(x*p) / p
}

func yearOf(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---------- matching ----------

// appleRel is what the matcher knows about one Discogs release.
type appleRel struct {
	id        int64
	title     string
	artist    string   // all credited artists, ", "-joined
	credits   []string // credited artist names
	creditIDs []int64
	various   bool
	year      int // original year, else this pressing's
	tracks    int // tracklist entries with a position
	durations []int
	barcodes  []string
}

type appleCand struct {
	appleItem
	via     string
	route   float64 // album listed under an artist found for one of the credits: 0.9 sure, 0.75 Apple's first answer
	score   float64
	t, a    float64
	dur     float64
	hasDur  bool
	barcode bool
}

func (r *appleRel) artistScore(c *appleCand) float64 {
	an := anorm(c.ArtistName)
	if r.various {
		if isVariousName(an) {
			return 1
		}
		best := 0.0
		for i, n := range r.credits {
			if r.creditIDs[i] != 194 {
				best = math.Max(best, asim(anorm(n), an))
			}
		}
		// a compilation filed under a performer: exact title, same track count and year
		cy := yearOf(c.ReleaseDate)
		if acore(r.title) == acore(c.CollectionName) && r.tracks > 0 && c.TrackCount > 0 &&
			iabs(r.tracks-c.TrackCount) <= 1 && r.year > 0 && iabs(cy-r.year) <= 2 {
			best = math.Max(best, 0.85)
		}
		return best
	}
	names := append(append([]string{}, r.credits...), r.artist)
	best := 0.0
	for _, n := range names {
		best = math.Max(best, asim(anorm(n), an))
	}
	for _, n := range names { // "Bowie" vs "David Bowie"
		if nn := anorm(n); nn != "" && an != "" && (hasWord(an, nn) || hasWord(nn, an)) {
			best = math.Max(best, 0.9)
		}
	}
	if isVariousName(an) { // the artist named in the title
		for _, n := range names {
			if nn := anorm(n); nn != "" && strings.Contains(anorm(c.CollectionName), nn) {
				best = math.Max(best, 0.9)
			}
		}
	}
	return math.Max(best, c.route)
}

func (r *appleRel) score(c *appleCand) {
	names := append(append(append([]string{}, r.credits...), r.artist), c.ArtistName)
	t := titleScore(r.title, c.CollectionName, names, isVariousName(anorm(c.ArtistName)))
	a := r.artistScore(c)
	s := 0.6*t + 0.4*a
	if a < 0.5 {
		// an alias the catalogue cannot know (Terence Trent D'Arby = Sananda Maitreya):
		// only a long, exactly equal title may pass
		if !(acore(r.title) == acore(c.CollectionName) && len([]rune(acore(r.title))) >= 25) {
			s = math.Min(s, 0.5)
		}
	}
	if reSingleEP.MatchString(c.CollectionName) && r.tracks > 6 {
		s -= 0.2
	}
	tb := 0.0
	if cy := yearOf(c.ReleaseDate); r.year > 0 && cy > 0 {
		tb -= float64(min(iabs(cy-r.year), 50)) * 0.0004
	}
	if r.tracks > 0 && c.TrackCount > 0 {
		tb -= float64(min(iabs(c.TrackCount-r.tracks), 30)) * 0.0005
	}
	c.score = round(math.Max(math.Min(s, 1), 0)+tb, 4)
	c.t, c.a = round(t, 3), round(a, 3)
}

// candidates keeps albums in the order first seen, as the scoring breaks ties by it.
type candidates struct {
	list []*appleCand
	byID map[int64]*appleCand
}

func (cs *candidates) add(c *appleCand) {
	if old, ok := cs.byID[c.CollectionID]; ok {
		old.route = math.Max(old.route, c.route)
		return
	}
	cs.byID[c.CollectionID] = c
	cs.list = append(cs.list, c)
}

func (cs *candidates) best(r *appleRel) float64 {
	b := 0.0
	for _, c := range cs.list {
		r.score(c)
		b = math.Max(b, c.score)
	}
	return b
}

type appleMatcher struct {
	api *appleAPI
	// per run: answers already fetched, so artists shared by records cost one request
	artistIDs map[string][]appleItem
	albums    map[string][]appleItem
}

func newAppleMatcher(api *appleAPI) *appleMatcher {
	return &appleMatcher{api: api, artistIDs: map[string][]appleItem{}, albums: map[string][]appleItem{}}
}

func (m *appleMatcher) searchArtists(ctx context.Context, name, country string) ([]appleItem, error) {
	key := country + "\x00" + name
	if v, ok := m.artistIDs[key]; ok {
		return v, nil
	}
	v, err := m.api.get(ctx, "/search", url.Values{"term": {name}, "entity": {"musicArtist"}, "attribute": {"artistTerm"},
		"country": {country}, "limit": {"5"}})
	if err == nil {
		m.artistIDs[key] = v
	}
	return v, err
}

func (m *appleMatcher) artistAlbums(ctx context.Context, id int64, country string, strength float64, via string) ([]*appleCand, error) {
	key := country + "\x00" + strconv.FormatInt(id, 10)
	v, ok := m.albums[key]
	if !ok {
		var err error
		v, err = m.api.get(ctx, "/lookup", url.Values{"id": {strconv.FormatInt(id, 10)}, "entity": {"album"}, "limit": {"200"}, "country": {country}})
		if err != nil {
			return nil, err
		}
		m.albums[key] = v
	}
	var out []*appleCand
	for _, x := range v {
		if x.WrapperType == "collection" {
			out = append(out, &appleCand{appleItem: x, via: via, route: strength})
		}
	}
	return out, nil
}

func nameMatches(an, nn string) bool {
	return asim(an, nn) >= 0.85 || (nn != "" && hasWord(an, nn)) || (an != "" && hasWord(nn, an))
}

// byArtist adds the albums of the artists found for one credited name (ids first..upto-1).
func (m *appleMatcher) byArtist(ctx context.Context, cs *candidates, name string, first, upto int) error {
	res, err := m.searchArtists(ctx, name, "ca")
	if err != nil {
		return err
	}
	type pick struct {
		id       int64
		strength float64
	}
	var ids []pick
	nn := anorm(name)
	for k, a := range res {
		if nameMatches(anorm(a.ArtistName), nn) {
			ids = append(ids, pick{a.ArtistID, 0.9})
		} else if k == 0 {
			ids = append(ids, pick{a.ArtistID, 0.75}) // Apple's first answer, e.g. 久石譲 -> Joe Hisaishi
		}
	}
	for i := first; i < upto && i < len(ids); i++ {
		cands, err := m.artistAlbums(ctx, ids[i].id, "ca", ids[i].strength, "artist albums")
		if err != nil {
			return err
		}
		for _, c := range cands {
			cs.add(c)
		}
	}
	return nil
}

func (m *appleMatcher) byArtistUS(ctx context.Context, cs *candidates, name string) error {
	res, err := m.searchArtists(ctx, name, "us")
	if err != nil {
		return err
	}
	nn := anorm(name)
	var picks []appleItem
	if len(res) > 0 {
		picks = append(picks, res[0])
		for _, x := range res[1:] {
			if asim(anorm(x.ArtistName), nn) >= 0.85 {
				picks = append(picks, x)
				break
			}
		}
	}
	for _, a := range picks {
		strength := 0.75
		if an := anorm(a.ArtistName); asim(an, nn) >= 0.85 || hasWord(an, nn) {
			strength = 0.9
		}
		cands, err := m.artistAlbums(ctx, a.ArtistID, "us", strength, "artist albums (US)")
		if err != nil {
			return err
		}
		for _, c := range cands {
			cs.add(c)
		}
	}
	return nil
}

func (m *appleMatcher) byTitle(ctx context.Context, cs *candidates, title, country string) error {
	seen := map[string]bool{}
	for _, v := range titleVariants(title) {
		t := acore(v)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		res, err := m.api.get(ctx, "/search", url.Values{"term": {t}, "entity": {"album"}, "attribute": {"albumTerm"},
			"country": {country}, "limit": {"200"}})
		if err != nil {
			return err
		}
		via := "album title search"
		if country == "us" {
			via += " (US)"
		}
		for _, x := range res {
			if x.WrapperType == "collection" {
				cs.add(&appleCand{appleItem: x, via: via})
			}
		}
	}
	return nil
}

var reStore = regexp.MustCompile(`music\.apple\.com/(\w\w)/`)

func storeOf(c *appleCand) string {
	if m := reStore.FindStringSubmatch(c.CollectionURL); m != nil {
		return m[1]
	}
	return "ca"
}

// songs fetches the track durations (seconds) of the given albums, 8 per request.
func (m *appleMatcher) songs(ctx context.Context, cands []*appleCand) (map[int64][]int, error) {
	byStore := map[string][]int64{}
	seen := map[int64]bool{}
	for _, c := range cands {
		if !seen[c.CollectionID] {
			seen[c.CollectionID] = true
			byStore[storeOf(c)] = append(byStore[storeOf(c)], c.CollectionID)
		}
	}
	out := map[int64][]int{}
	stores := make([]string, 0, len(byStore))
	for s := range byStore {
		stores = append(stores, s)
	}
	sort.Strings(stores)
	for _, s := range stores {
		ids := byStore[s]
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for i := 0; i < len(ids); i += 8 {
			chunk := ids[i:min(i+8, len(ids))]
			var parts []string
			for _, id := range chunk {
				parts = append(parts, strconv.FormatInt(id, 10))
			}
			res, err := m.api.get(ctx, "/lookup", url.Values{"id": {strings.Join(parts, ",")}, "entity": {"song"}, "limit": {"200"}, "country": {s}})
			if err != nil {
				return nil, err
			}
			for _, x := range res {
				if x.WrapperType == "track" && x.TrackTimeMillis > 0 {
					out[x.CollectionID] = append(out[x.CollectionID], int(math.RoundToEven(float64(x.TrackTimeMillis)/1000)))
				}
			}
		}
	}
	return out, nil
}

// durationRatio is the share of Discogs durations with an unused Apple track within max(5 s, 3 %).
func durationRatio(ds, apple []int) float64 {
	free := append([]int{}, apple...)
	hit := 0
	for _, d := range ds {
		tol := math.Max(5, float64(d)*0.03)
		best := -1
		for i, a := range free {
			if math.Abs(float64(a-d)) <= tol && (best < 0 || iabs(a-d) < iabs(free[best]-d)) {
				best = i
			}
		}
		if best >= 0 {
			free = append(free[:best], free[best+1:]...)
			hit++
		}
	}
	return float64(hit) / float64(len(ds))
}

func scriptOf(t string) string {
	letters, latin := 0, 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			letters++
			if unicode.Is(unicode.Latin, r) {
				latin++
			}
		}
	}
	if letters == 0 || float64(latin)/float64(letters) > 0.5 {
		return "latin"
	}
	return "other"
}

var reNonWordSpace = regexp.MustCompile(`[^\p{L}\p{N}_ ]`)

func plainArtist(a string) string {
	return reNonWordSpace.ReplaceAllString(strings.Join(strings.Fields(strings.ToLower(a)), " "), " ")
}

// namedMatch says whether the Apple artist is one of the record's credited
// artists (or Various Artists for a compilation).
func (r *appleRel) namedMatch(appleArtist string) bool {
	aa := plainArtist(appleArtist)
	if r.various && isVariousName(strings.TrimSpace(aa)) {
		return true
	}
	stop := map[string]bool{"the": true, "and": true, "of": true}
	aw := map[string]bool{}
	for _, w := range strings.Fields(aa) {
		if !stop[w] {
			aw[w] = true
		}
	}
	for _, n := range r.credits {
		if strings.ToLower(n) == "various" {
			continue
		}
		nn := plainArtist(n)
		if simRatio(nn, aa) >= 0.7 {
			return true
		}
		for _, w := range strings.Fields(nn) {
			if aw[w] {
				return true
			}
		}
		if scriptOf(n) != scriptOf(appleArtist) {
			return true // 久石譲 / Joe Hisaishi: cannot compare, do not punish
		}
	}
	return false
}

// match finds the album for one release.
func (m *appleMatcher) match(ctx context.Context, r *appleRel) (AppleMatch, error) {
	cs := &candidates{byID: map[int64]*appleCand{}}
	var upc *appleCand
barcodes:
	for _, b := range r.barcodes {
		for _, country := range []string{"ca", "us"} {
			res, err := m.api.get(ctx, "/lookup", url.Values{"upc": {b}, "country": {country}})
			if err != nil {
				return AppleMatch{}, err
			}
			for _, x := range res {
				if x.WrapperType == "collection" {
					upc = &appleCand{appleItem: x, via: "barcode " + b}
					cs.add(upc)
					break barcodes
				}
			}
		}
	}
	credits := func(f func(name string) error) error {
		for i, n := range r.credits {
			if r.creditIDs[i] == 194 {
				continue
			}
			if err := f(n); err != nil {
				return err
			}
		}
		return nil
	}
	if err := credits(func(n string) error { return m.byArtist(ctx, cs, n, 0, 2) }); err != nil {
		return AppleMatch{}, err
	}
	best := cs.best(r)
	if upc == nil && best < 0.85 {
		if err := credits(func(n string) error { return m.byArtist(ctx, cs, n, 2, 5) }); err != nil {
			return AppleMatch{}, err
		}
		best = cs.best(r)
	}
	if upc == nil && best < 0.85 {
		if err := m.byTitle(ctx, cs, r.title, "ca"); err != nil {
			return AppleMatch{}, err
		}
		best = cs.best(r)
	}
	if upc == nil && best < 0.85 {
		if err := credits(func(n string) error { return m.byArtistUS(ctx, cs, n) }); err != nil {
			return AppleMatch{}, err
		}
		if err := m.byTitle(ctx, cs, r.title, "us"); err != nil {
			return AppleMatch{}, err
		}
		cs.best(r)
	}
	pool := append([]*appleCand{}, cs.list...)
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].score > pool[j].score })
	if len(pool) > 60 {
		pool = pool[:60]
	}
	status := "none"
	if upc != nil {
		status = "barcode"
	} else if len(pool) > 0 && pool[0].score >= 0.85 {
		status = "found"
	}

	// track durations
	if len(r.durations) >= 3 && len(pool) > 0 {
		picks := pool[:1]
		if upc == nil && status != "found" {
			n := 0
			for _, c := range pool[min(5, len(pool)):] {
				if n == 10 {
					break
				}
				if (c.route > 0 || strings.Contains(c.via, "title")) && c.TrackCount > 0 && iabs(c.TrackCount-r.tracks) <= 3 {
					picks = append(picks, c)
					n++
				}
			}
			picks = append(picks, pool[1:min(4, len(pool))]...)
		}
		songs, err := m.songs(ctx, picks)
		if err != nil {
			return AppleMatch{}, err
		}
		for _, c := range pool {
			if s, ok := songs[c.CollectionID]; ok {
				c.dur, c.hasDur = round(durationRatio(r.durations, s), 2), true
			}
		}
	}
	for _, c := range pool {
		c.barcode = upc != nil && c.CollectionID == upc.CollectionID
		s := c.score
		if c.hasDur {
			if c.dur >= 0.8 && c.a >= 0.75 && (c.t >= 0.6 || scriptOf(r.title) != scriptOf(c.CollectionName)) {
				s = math.Max(s, 0.9) + 0.05 // same tracks, same artist: the album, whatever its title says
			} else if c.dur < 0.3 && r.tracks > 0 && iabs(c.TrackCount-r.tracks) <= 2 {
				s -= 0.25 // comparable tracklists, almost no track lasts as long: another album
			}
		}
		if !c.barcode && !(c.hasDur && c.dur >= 0.8) && !r.namedMatch(c.ArtistName) {
			// Apple files the album under someone not credited on the record (a recording
			// listed under its composer, a compilation under one performer): durations must confirm it
			s = math.Min(s, 0.8)
		}
		if r.various && isVariousName(strings.Join(strings.Fields(strings.ToLower(c.ArtistName)), " ")) &&
			r.tracks > 0 && c.TrackCount > 0 && iabs(c.TrackCount-r.tracks) > 1 {
			s = math.Min(s, 0.8) // a compilation with another track count is another compilation
		}
		c.score = round(s, 4)
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].score > pool[j].score })
	out := AppleMatch{Status: "none", CheckedAt: time.Now().UTC()}
	var best2 *appleCand
	if upc != nil {
		for _, c := range pool {
			if c.barcode {
				best2 = c
			}
		}
		out.Status = "barcode"
	} else if len(pool) > 0 {
		best2 = pool[0]
		switch {
		case best2.score >= 0.85:
			out.Status = "found"
		case best2.score >= 0.65:
			out.Status = "unsure"
		}
	}
	if best2 != nil {
		out.Album, out.Score = best2.CollectionID, best2.score
	}
	return out, nil
}

// ---------- sync ----------

// discogsSeconds turns the release's tracklist durations into seconds (entries with a position only).
func discogsSeconds(ts []Track) []int {
	var out []int
	for _, t := range ts {
		if t.Position == "" || t.Heading {
			continue
		}
		p := strings.Split(t.Duration, ":")
		ok := len(p) == 2 || len(p) == 3
		for _, x := range p {
			ok = ok && allDigits(x)
		}
		if !ok {
			continue
		}
		v := 0
		for _, x := range p {
			n, _ := strconv.Atoi(x)
			v = v*60 + n
		}
		out = append(out, v)
	}
	return out
}

func countTracks(ts []Track) int {
	n := 0
	for _, t := range ts {
		if t.Position != "" && !t.Heading {
			n++
		}
	}
	return n
}

func appleRelFor(d *Data, r *Release, det *Details) *appleRel {
	ar := &appleRel{id: r.ID, title: r.Title, various: r.compilation()}
	for _, a := range r.Artists {
		ar.credits = append(ar.credits, a.Name)
		ar.creditIDs = append(ar.creditIDs, a.ID)
	}
	ar.artist = strings.Join(ar.credits, ", ")
	ar.year = r.Year
	if y := d.MasterYears[r.MasterID]; r.MasterID != 0 && y != 0 {
		ar.year = y
	}
	ar.tracks = countTracks(det.Tracklist)
	ar.durations = discogsSeconds(det.Tracklist)
	ar.barcodes = det.Barcodes
	return ar
}

// matchApple finds Apple Music albums for releases that have none yet, and
// tries again every appleRecheck for those without a link. Manual albums are
// never searched. It waits for the release details (barcodes, tracklist).
func (a *App) matchApple(ctx context.Context, releases []Release) error {
	if a.apple == nil {
		return nil
	}
	var todo []*appleRel
	seen := map[int64]bool{}
	a.store.view(func(d *Data) {
		for i := range releases {
			r := &releases[i]
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			if _, ok := appleManual[r.ID]; ok {
				continue
			}
			det, ok := d.Details[r.ID]
			if !ok || !det.IDsRead {
				continue
			}
			m := d.Apple[r.ID]
			if m != nil && (m.linked() || time.Since(m.CheckedAt) < appleRecheck) {
				continue
			}
			todo = append(todo, appleRelFor(d, r, det))
		}
	})
	if err := a.store.update(func(d *Data) {
		for id, album := range appleManual {
			if m := d.Apple[id]; m == nil || m.Status != "manual" || m.Album != album {
				d.Apple[id] = &AppleMatch{Album: album, Status: "manual", CheckedAt: time.Now().UTC()}
			}
		}
	}); err != nil {
		return err
	}
	if len(todo) == 0 {
		return nil
	}
	start := time.Now()
	m := newAppleMatcher(a.apple)
	links := 0
	defer a.store.update(func(*Data) {}) // keep what was found before an error or shutdown
	for i, r := range todo {
		res, err := m.match(ctx, r)
		if err != nil {
			return fmt.Errorf("apple music %d: %w", r.id, err)
		}
		if (&res).linked() {
			links++
		}
		a.store.mu.Lock()
		if _, manual := appleManual[r.id]; !manual {
			a.store.data.Apple[r.id] = &res
		}
		a.store.mu.Unlock()
		if (i+1)%10 == 0 {
			if err := a.store.update(func(*Data) {}); err != nil {
				return err
			}
		}
	}
	log.Printf("sync: Apple Music searched for %d releases in %s, %d linked", len(todo), time.Since(start).Round(time.Second), links)
	return nil
}
