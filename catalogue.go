package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// joinArtists is the credit line as Discogs prints it: "Daryl Hall & John
// Oates", "Electric Light Orchestra / Olivia Newton-John".
func joinArtists(as []Artist) string {
	var b strings.Builder
	for i, a := range as {
		b.WriteString(a.Name)
		if i == len(as)-1 {
			break
		}
		switch j := a.Join; {
		case j == "" || j == ",":
			b.WriteString(j + " ")
		default:
			b.WriteString(" " + j + " ")
		}
	}
	return strings.TrimSpace(b.String())
}

// sortKey orders artists the way record shops file them: case and accents
// ignored, a leading "The " dropped (The Beatles under B).
func sortKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "the ")
	return foldAccents.Replace(s)
}

// foldAccents maps the accented Latin letters seen in artist names to plain
// ones (the standard library has no Unicode normalisation).
var foldAccents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ñ", "n", "ç", "c", "ý", "y", "ÿ", "y", "š", "s", "ž", "z", "č", "c",
	"ё", "е",
)

// artistSort is the sort key of a release's first credited artist.
func (r *Release) artistSort() string {
	if len(r.Artists) == 0 {
		return ""
	}
	return sortKey(r.Artists[0].Sort)
}

func (r *Release) artistLine() string { return joinArtists(r.Artists) }

// formatLines are Discogs-style lines: "1 × Vinyl, LP, Album, Reissue, Orange".
func (r *Release) formatLines() []string {
	var out []string
	for _, f := range r.Formats {
		parts := []string{}
		head := f.Name
		if f.Qty != "" {
			head = f.Qty + " × " + f.Name
		}
		parts = append(parts, head)
		parts = append(parts, f.Descriptions...)
		if f.Text != "" {
			parts = append(parts, f.Text)
		}
		out = append(out, strings.Join(parts, ", "))
	}
	return out
}

// formatShort is a compact format for cards: "LP", "2×LP", "CD", "7\"".
func (r *Release) formatShort() string {
	var out []string
	for _, f := range r.Formats {
		name := f.Name
		for _, d := range f.Descriptions {
			switch d {
			case "LP", "EP", "12\"", "10\"", "7\"", "Single", "Maxi-Single":
				name = d
			}
			if name != f.Name {
				break
			}
		}
		if name == "All Media" || name == "Box Set" {
			continue
		}
		if q, _ := strconv.Atoi(f.Qty); q > 1 {
			name = strconv.Itoa(q) + "×" + name
		}
		out = append(out, name)
	}
	return strings.Join(out, " + ")
}

// tags are genres then styles, without repeats.
func (r *Release) tags() []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range append(append([]string{}, r.Genres...), r.Styles...) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func (r *Release) primaryGenre() string {
	if len(r.Genres) > 0 {
		return r.Genres[0]
	}
	return ""
}

// originalYear is the master release's year. A release without a master is
// its own original; while the master is not read yet, the year is 0
// (unknown) rather than this pressing's.
func originalYear(d *Data, r *Release) int {
	if r.MasterID == 0 {
		return r.Year
	}
	return d.MasterYears[r.MasterID]
}

// sortYear is the original year when known, else this pressing's.
func sortYear(d *Data, r *Release) int {
	if y := originalYear(d, r); y > 0 {
		return y
	}
	return r.Year
}

var months = map[string][]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"ru": {"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"},
}

var monthsNom = map[string][]string{
	"en": months["en"],
	"ru": {"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"},
}

func formatDate(lang string, t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if lang == "ru" {
		return fmt.Sprintf("%d %s %d", t.Day(), months["ru"][t.Month()-1], t.Year())
	}
	return t.Format("January 2, 2006")
}

// formatReleased turns Discogs' "2023-09-15", "1985-05-00", "1985-00-00" or
// "1985" into a date with only the known parts.
func formatReleased(lang, s string) string {
	parts := strings.Split(s, "-")
	y, _ := strconv.Atoi(parts[0])
	if y == 0 {
		return ""
	}
	var m, d int
	if len(parts) > 1 {
		m, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		d, _ = strconv.Atoi(parts[2])
	}
	switch {
	case m < 1 || m > 12:
		return strconv.Itoa(y)
	case d == 0:
		return monthsNom[lang][m-1] + " " + strconv.Itoa(y)
	}
	return formatDate(lang, time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC))
}

var (
	reNamedLink = regexp.MustCompile(`\[(?:a|l|m|r)=([^\]]+)\]`)
	reIDLink    = regexp.MustCompile(`\[(?:a|l|m|r)\d+\]`)
	reURL       = regexp.MustCompile(`(?s)\[url=[^\]]*\](.*?)\[/url\]`)
	reTag       = regexp.MustCompile(`\[/?(?:b|i|u|s)\]`)
)

// cleanNotes turns Discogs markup into plain text paragraphs.
func cleanNotes(s string) []string {
	s = reURL.ReplaceAllString(s, "$1")
	s = reNamedLink.ReplaceAllStringFunc(s, func(m string) string {
		return artistName(reNamedLink.FindStringSubmatch(m)[1])
	})
	s = reIDLink.ReplaceAllString(s, "")
	s = reTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out []string
	for _, p := range strings.Split(s, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// clientRelease is one record in the JSON the page script works from.
type clientRelease struct {
	I int64  `json:"i"` // collection instance
	T string `json:"t"` // title
	A string `json:"a"` // artist line
	S string `json:"s"` // artist sort key
	Y int    `json:"y"` // this pressing's year
	O int    `json:"o"` // original year, 0 while unknown
	G string `json:"g"` // primary genre
	F string `json:"f"` // short format
	C string `json:"c"` // cover URL on this site, "" without one
	Q string `json:"q"` // extra search text: tags, labels, catalogue numbers
}

func (a *App) clientReleases(d *Data) []clientRelease {
	out := make([]clientRelease, 0, len(d.Releases))
	for i := range d.Releases {
		r := &d.Releases[i]
		var q []string
		q = append(q, r.tags()...)
		for _, l := range r.Labels {
			q = append(q, l.Name, l.Catno)
		}
		out = append(out, clientRelease{
			I: r.Instance, T: r.Title, A: r.artistLine(), S: r.artistSort(), Y: r.Year, O: originalYear(d, r),
			G: r.primaryGenre(), F: r.formatShort(), C: a.coverURL(d, r), Q: strings.Join(q, " "),
		})
	}
	return out
}

// byArtist orders releases by artist, then original year, then title.
func byArtist(d *Data, rs []*Release) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if ka, kb := a.artistSort(), b.artistSort(); ka != kb {
			return ka < kb
		}
		if ya, yb := sortYear(d, a), sortYear(d, b); ya != yb {
			return ya < yb
		}
		return a.Title < b.Title
	})
}
