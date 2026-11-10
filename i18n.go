package main

import "strconv"

func releasesCount(n int) string {
	if n == 1 {
		return "1 release"
	}
	return strconv.Itoa(n) + " releases"
}

var texts = map[string]string{
	"collection":    "Record collection",
	"collection_of": "%s's record collection",
	"search":        "Search the collection",
	"group_by":      "Stacks by",
	"g_artist":      "Artist",
	"g_genre":       "Genre",
	"g_decade":      "Decade",
	"g_year":        "Original year",
	"view_stacks":   "Stacks",
	"view_wall":     "Wall",
	"random":        "Random",
	"theme":         "Theme",
	"theme_auto":    "Theme: follows the system",
	"theme_light":   "Theme: day",
	"theme_dark":    "Theme: night",
	"back":          "Back to the collection",
	"prev":          "Previous",
	"next":          "Next",
	"close":         "Close",
	"of":            "of",
	"nothing":       "Nothing found",
	"original_year": "Original year",
	"version_year":  "This pressing",
	"released":      "Released",
	"country":       "Country",
	"label":         "Label",
	"catno":         "Catalogue no.",
	"format":        "Format",
	"prev_record":   "Previous record",
	"next_record":   "Next record",
	"enter_stack":   "Dig into this stack",
	"order_asc":     "Normal order",
	"order_desc":    "Reverse order",
	"tracklist":     "Tracklist",
	"notes":         "Notes",
	"on_discogs":    "On Discogs",
	"details_wait":  "Tracklist and notes are still being read from Discogs.",
	"attribution":   "Data provided by Discogs",
	"not_found":     "There is no such record in the collection.",
	"empty":         "The collection is being read from Discogs. Refresh the page in a minute.",
	"other":         "Other",
}

// t returns the interface text for key (the site is English only).
func t(key string) string { return texts[key] }
