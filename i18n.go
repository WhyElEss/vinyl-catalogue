package main

import (
	"net/http"
	"strconv"
	"strings"
)

// detectLang: the "lang" cookie, else the first of ru/en in Accept-Language,
// else English.
func detectLang(r *http.Request) string {
	if c, err := r.Cookie("lang"); err == nil && (c.Value == "ru" || c.Value == "en") {
		return c.Value
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(strings.ToLower(part)), ";")
		switch {
		case strings.HasPrefix(tag, "ru"):
			return "ru"
		case strings.HasPrefix(tag, "en"):
			return "en"
		}
	}
	return "en"
}

// pluralRU picks the Russian noun form for n (Unicode CLDR one/few/many).
func pluralRU(n int, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	switch {
	case n%10 == 1 && n%100 != 11:
		return one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return few
	}
	return many
}

func releasesCount(lang string, n int) string {
	if lang == "ru" {
		return strconv.Itoa(n) + " " + pluralRU(n, "релиз", "релиза", "релизов")
	}
	if n == 1 {
		return "1 release"
	}
	return strconv.Itoa(n) + " releases"
}

var texts = map[string]map[string]string{
	"en": {
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
	},
	"ru": {
		"collection":    "Коллекция пластинок",
		"collection_of": "Коллекция пластинок: %s",
		"search":        "Поиск по коллекции",
		"group_by":      "Стопки",
		"g_artist":      "По исполнителю",
		"g_genre":       "По жанру",
		"g_decade":      "По десятилетию",
		"g_year":        "По году оригинала",
		"view_stacks":   "Стопки",
		"view_wall":     "Стена",
		"random":        "Случайная",
		"theme":         "Тема",
		"theme_auto":    "Тема: как в системе",
		"theme_light":   "Тема: дневная",
		"theme_dark":    "Тема: ночная",
		"back":          "Назад к коллекции",
		"prev":          "Предыдущая",
		"next":          "Следующая",
		"close":         "Закрыть",
		"of":            "из",
		"nothing":       "Ничего не найдено",
		"original_year": "Год оригинала",
		"version_year":  "Это издание",
		"released":      "Дата выхода",
		"country":       "Страна",
		"label":         "Лейбл",
		"catno":         "Каталожный номер",
		"format":        "Формат",
		"prev_record":   "Предыдущая пластинка",
		"next_record":   "Следующая пластинка",
		"enter_stack":   "Перебрать стопку",
		"order_asc":     "Прямой порядок",
		"order_desc":    "Обратный порядок",
		"tracklist":     "Треклист",
		"notes":         "Примечания",
		"on_discogs":    "На Discogs",
		"details_wait":  "Треклист и примечания ещё загружаются из Discogs.",
		"attribution":   "Данные предоставлены Discogs",
		"not_found":     "Такой пластинки в коллекции нет.",
		"empty":         "Коллекция загружается из Discogs. Обновите страницу через минуту.",
		"other":         "Другое",
	},
}

func t(lang, key string) string {
	if s, ok := texts[lang][key]; ok {
		return s
	}
	return texts["en"][key]
}
