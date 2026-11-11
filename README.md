# vinyl-catalogue

A read-only showcase of one Discogs user's record collection. One Go binary built only on the standard library, all state in one JSON file plus a folder of covers, runs in Docker on a Raspberry Pi behind a Cloudflare Tunnel.

There is no admin panel, no accounts and nothing social: the collection is edited on Discogs, and the site reads it from the Discogs API on its own.

The site is in English only, with a day and a night theme.

## Features

- **Stacks.** The collection stands in piles on a horizontal rail, like crates in a record shop: by artist initial (default), genre, decade, or year of the original release. Each pile shows, behind its front, the tops of the other sleeves — the nearest one exactly as wide as the cover, the pile narrowing by at most 1% a side toward the back; a pile of one record has none.
- **Crate lids.** A closed pile wears a lid of smoked plexiglass over its first cover, with the pile's name (letter, decade, year, VARIOUS) cut through it in Jost Bold, a polished edge round every cut, the shadow of the plexiglass's thickness inside the letters and the finger slot, and a plain finger slot near the top. A real lid would lose the middles of letters such as O, A, R or B, so the page finds every such island and holds it with thin bridges up and down from its centre. Under a closed pile is only "15 releases". Entering the pile drops the lid forward; leaving it lifts it back.

  The lid is drawn in the browser as an SVG (the cover and the font embedded) and, once the row stands still, replaced by a JPEG of itself: WebKit re-runs the SVG's blur and masks on every paint, a bitmap is only copied. Measured in Safari on a MacBook: a JPEG takes about 23 ms to encode and 66 KB (a PNG 37 ms and 521 KB; the lid has no transparency to keep). The label's geometry (font size, bridges) takes about 30 ms per label (drawn at 1×; at 2× it took 91–130 ms) and is kept in `localStorage`. The lids nearest the middle are made first; two seconds after the page has loaded, every other lid is made too, one at a time while the row stands still, so a pile never arrives as a black square; while the row moves only the lid about to be seen is made, at most every 150 ms.
- **Crate digging.** The pile in the middle is entered explicitly (↓, Enter or a click; its lid drops in 0.42 s, none with "reduce motion" on) and then dug through in place: the front record tilts forward and falls toward you, uncovering the next one. The records already flipped lie flat in front of the pile, below the front record and never over its cover, drawn as plain edges (a face-down cover would not show) — so the fallen pile grows as the pile behind shrinks; before the first flip there is none. Every pile has the same height and a fixed label box, so the row never moves when a long artist or title wraps. The label under the pile shows artist, title and "3 of 15". The piles on either side stay piles, all of one size, and → or ← closes the open pile and moves to its neighbour without opening it.

  | | Keyboard | Mouse, trackpad | Touch |
  |---|---|---|---|
  | Next / previous pile | → / ← | arrow buttons, a click on a side pile, the index under the rail, a sideways swipe on a trackpad (fingers left = →) | swipe left for the next pile (→), right for the previous one (←); one pile per swipe |
  | Start digging the middle pile | ↓ or Enter | a click on it; wheel or two fingers down | tap; finger down |
  | Next / previous record (while digging) | ↓ / ↑ | the ⌄ / ⌃ buttons under the pile; wheel or two fingers down / up | finger down / up |
  | Open the front record (while digging) | Enter | a click on it | tap |
  | Close the record, then stop digging | Esc, Esc (↑ on the first record also stops digging) | ✕ or a click outside; wheel or two fingers up on the first record | finger up on the first record |

  Every gesture is a key press, on every system, and one swipe is one press — the same function the keys call, with the same animations. Down and up: a wheel, two fingers on a trackpad or one finger on a screen moving **down** is ↓, moving **up** is ↑ (on Apple devices, where scrolling is "natural" by default, the browser reports a downward swipe as scrolling up, so the direction is turned there; a Mac with natural scrolling switched off gets it reversed — the browser cannot see that setting). Sideways, on a trackpad and on a touch screen alike, it works like turning a page: fingers moving **left** bring the next pile (→), **right** the previous one (←). The row never scrolls by itself. A trackpad does not tell the page where a swipe begins: it sends one stream of small moves about 8 ms apart, the swipe followed by its inertia. Recorded on a MacBook in Safari, a new swipe always begins after a break of at least 27 ms in that stream, even when it lands on the previous swipe's inertia, while a swipe and its inertia never break for 20 ms. So a 22 ms break starts a new swipe, and the swipe presses its key as soon as it has moved 30 px — no waiting. Right after a press Safari holds the trackpad events back for 30–60 ms while the page starts its animation and then hands them over at once, which looks like such a break; so no new swipe starts within 150 ms of a press (measured: those false second presses came 18–79 ms after the first, real swipes at least 440 ms apart). The row is moved by the page's own frame-by-frame animation (0.38 s), not by the browser's smooth scrolling, which Safari cancels whenever trackpad events arrive; it has no scroll snapping either, since nothing but this animation moves it. Quick presses or swipes count from the pile the row is already heading for, so none gets lost while the row is still moving.

  The address keeps the place (`?s=J&r=2` is the third record of pile J), so a reload or a shared link opens the same record at the front.
- **Phones.** Order and Random are icons only, so all the controls take one row under the search. The sleeve edges and the fallen pile are drawn at 0.6 scale, and the cover you dig through is sized from the screen height, so the record's artist, title and buttons stay above the browser bar that Safari on iOS floats over the bottom of the page.
- **Wall.** Every cover in a grid, under the same grouping.
- **Normal or reverse order** for any grouping, in stacks and on the wall: the button reverses both the order of the stacks and the records in each one (Z…A, newest first), keeps the same stack in the middle, and is kept in the address (`?o=desc`).
- **Order.** Artists are filed the way record shops do it: case and accents are ignored, and a leading "The" is dropped (The Beatles stand under B). Records of one artist go by the year of the original release (the Discogs master), then by title. Latin initials come first, then Cyrillic, then everything else under "#". Compilations (the Discogs artist "Various") stand apart, in a stack of their own after "#" when stacked by artist, and go by title, then year; in the other groupings they come after the rest of their stack. Numbers in titles count as numbers ("Vol. 2" before "Vol. 10").
- **Sort as.** Where the rules put a record in the wrong place (two parts of one release with different titles, say), the owner can add a text field named **Sort as** to their Discogs collection fields and fill it in for that record: its value takes the place of the title in sorting. It is read with the owner's token (`DISCOGS_TOKEN`), as Discogs shows private collection fields only to the owner, and picked up at the next sync. Only this field is read; the owner's other collection fields (condition, notes and so on) are neither stored nor shown, and the page itself gets only each record's place in the order, not the text.
- **Search** over artist, title, genres and styles, labels, catalogue numbers, format and years; every word must match.
- **Random** opens a random record (from the search results, if there is a search).
- **Release card** (an overlay on the collection page, and a page of its own at `/release/<id>/<slug>` for direct links): cover, artist, title, original year, genres and styles, labels and catalogue numbers, format as Discogs writes it ("1 × Vinyl, LP, Album, Reissue, Orange"), this pressing's release date and country, the tracklist with headings and durations, the Discogs release notes (folded), and a link to the release on Discogs. The overlay grows out of the cover that was chosen (the front record of a pile, a card on the wall): its own cover starts exactly over the small one and the rest of the card fades in around it; closing (×, Esc, a click outside) shrinks it back into that cover. Without a cover on screen (Random, Forward) it rises in and sinks out; none with "reduce motion" on.
- **Nothing personal.** The site keeps and shows only what Discogs says about the releases: no date added, no rating, no condition, no notes, no profile.
- **Themes.** The theme follows the system until the visitor picks one with the button in the header (system → day → night); the choice is kept in the browser.
- **Link previews**: the collection page shares a 1200×630 card (`static/og.png`: "My record collection — Dig through the crates", the domain, a record sliding out of its sleeve) with the owner's name and the number of releases in the text; a record's page shares its own cover, title, original year and format. Full Open Graph and Twitter tags, plus `favicon.ico`, `favicon-32.png`, `icon.svg` and a 180×180 `apple-touch-icon.png` for Safari Favorites and the iOS home screen.
- **Light first load.** Only the covers of the piles within three steps of the middle are fetched (one more pile's covers as the row moves); the plain list of all records for browsers without JavaScript sits in `<noscript>`, so other browsers never fetch its covers. Covers are kept at most 600 px on a side (Discogs' API serves no more) and re-encoded as JPEG quality 75 with Go's own encoder, keeping the original when that is not smaller: the 319 covers went from 34.6 MB to 16.9 MB. Covers downloaded before this are re-encoded once at start (marker file `covers/.optimized-1`); a cover's URL includes its file size, so browsers fetch the new file.
- Open to search engines: `robots.txt` allows everything and points to `/sitemap.xml`, which lists the collection page and every record's own page; link-preview crawlers read the pages too.

## How the collection is read

There are two kinds of sync:

- **At start** (every container start): the collection list, and whatever is still missing — covers, release details, original years. New records appear this way after a restart.
- **Daily** (`SYNC_INTERVAL`, 24 hours after the last one; at start too when the last one is older): the list and **every** release again. Edited tracklists and notes arrive this way, and so do covers: when someone uploads a better scan to Discogs and it becomes the release's primary image, the release names a different image URL (the URL carries the stored file's name, e.g. `R-31012123-1718895704-8819.jpeg`, with the upload time in it), the new file is downloaded and replaces ours, and the page asks browsers for it under a new `?v=`.

| What | Discogs API | When |
|---|---|---|
| Owner's name | `GET /users/{user}` | Every sync |
| Collection list (folder 0, "All") | `GET /users/{user}/collection/folders/0/releases`, 100 per page | Every sync |
| Tracklist, notes, country, release date, primary image | `GET /releases/{id}` | Missing ones at start; all of them daily |
| Cover | the release's primary image (Discogs serves up to 600 px), from `i.discogs.com`; the list's `cover_image` until the release is read | When missing or when the image URL changed |
| Original year | `GET /masters/{id}` (`year`) | Once per master |

The collection must be public on Discogs; the site reads it without signing in. Discogs allows 25 requests a minute without a token and 60 with one, counted over a moving minute; the app spaces its requests so it stays under the limit and waits a minute on a 429. Measured on the Pi with the 320 records of the first deployment and no token: the list takes about 10 seconds; reading 249 releases with their masters took 22 min 48 s, so the very first sync (every release and master) takes about half an hour; a daily refresh of all 319 releases took 13 min 28 s. The list and covers appear within the first minute; tracklists and original years fill in as they are read, newest records first, and the data file is saved every 10 releases.

Custom collection fields (media and sleeve condition, your own notes) are not part of the public API answer and are not shown.

Covers and data are served from the app itself, so the pages load nothing from other sites (the Content Security Policy allows only the site's own origin).

## Architecture

```
 visitors ──https──▶ Cloudflare ──tunnel──▶ cloudflared ──http://vinyl:8097──▶ vinyl-catalogue (Go)
                                                                                   │
                                        api.discogs.com, i.discogs.com ◀───────────┤
                                                                                   ▼
                                                          /data/catalogue.json, /data/covers/
```

| File | What it does |
|---|---|
| `main.go`, `config.go` | Start-up, configuration from the environment |
| `discogs.go` | Discogs API client: request spacing, retry on 429, response shapes |
| `sync.go` | The sync loop (at start and daily): collection list, covers, release details, master years |
| `store.go` | Data model and the JSON store: one lock, atomic file replace |
| `catalogue.go` | Artist names and sort keys, format lines, dates, notes markup, the JSON for the page script |
| `release.go` | Release page and the overlay fragment (`?part=1`) |
| `app.go` | Routes, security headers, rendering, covers |
| `i18n.go` | Interface texts (English) |
| `templates/` | Page layout, collection page, release card |
| `static/app.js` | Stacks and crate digging, wall, search, grouping, overlay, keyboard, wheel and touch |
| `static/fonts/` | Jost Bold (latin and cyrillic), SIL Open Font License (`Jost-OFL.txt`), used by the lids |
| `static/theme.js` | Theme choice, loaded in `<head>` so the page never flashes the other theme |

`catalogue.json` holds the owner, the collection list (one entry per copy, so a record owned twice appears twice), release details keyed by Discogs release id, and original years keyed by master id. Details and years outlive the records that needed them, so a record removed and added back costs no new requests.

## Running on a server

The host keeps only `docker-compose.yml` and `.env`; CI publishes the image as `ghcr.io/whyeless/vinyl-catalogue:<7-character commit>` for linux/arm64 and linux/amd64.

```bash
mkdir ~/vinyl && cd ~/vinyl
# put docker-compose.yml from this repository here, then:
cp .env.example .env   # or write it by hand; see Configuration
chmod 600 .env
docker compose pull && docker compose up -d
```

The site is at `http://<host>:8097`.

Update: set `VINYL_TAG` in `.env` to the new commit, then `docker compose pull && docker compose up -d`.

### Public hostname (Cloudflare Tunnel)

The tunnel runs as a separate Compose project, `~/cloudflared-<name>`, with `TUNNEL_TOKEN` in its own `.env` (mode 600). It joins `vinyl_net` and reaches the app as `http://vinyl:8097`, so the host port is not needed for it:

```yaml
services:
  cloudflared:
    image: cloudflare/cloudflared:latest
    container_name: cloudflared-<name>
    restart: always
    command: tunnel --no-autoupdate run
    environment:
      TUNNEL_TOKEN: ${TUNNEL_TOKEN}
    networks:
      - vinyl_net

networks:
  vinyl_net:
    external: true
```

In Cloudflare: **Networking → Tunnels → Create a tunnel**, copy the token, then **Routes → Add route → Published application** with the service URL `http://vinyl:8097`. Set `BASE_URL` in the app's `.env` to the public origin.

## Configuration

See [.env.example](.env.example).

| Variable | Meaning |
|---|---|
| `DISCOGS_USER` | Required. Whose collection is shown |
| `DISCOGS_TOKEN` | Optional personal access token of the collection owner: raises the API limit from 25 to 60 requests a minute and lets the site read the private **Sort as** collection field |
| `SYNC_INTERVAL` | How often every release is read again (Go duration, at least `1h`; default `24h`) |
| `BASE_URL` | Public origin, for link previews |
| `HOST_PORT` | Host port for the LAN (default 8097) |
| `TZ` | Time zone for dates and logs |
| `VINYL_TAG` | Image tag Compose runs |

## Development

There are no dependencies beyond the Go standard library. CI runs `gofmt`, `go vet`, `go test -race` and a `linux/arm64` build on every push, and publishes the image on every push to `main`.

```bash
go test ./...
DATA_DIR=./data DISCOGS_USER=<user> go run .
```

### Debugging input

`/?debug=wheel` shows a panel in the top right corner listing every wheel event (time since the previous one, deltaX, deltaY) and every key press with its source; **Copy log** and **Save log** take the whole log. On a test copy started with `DEBUG_LOG=1` the page also posts the log every 2 s to `POST /debug/wheel-log`, which keeps it as `debug-wheel-log.json` in the data directory. Without `DEBUG_LOG` that route does not exist.

## License

No license has been chosen yet for the code. The Jost font in `static/fonts/` (copyright 2020 The Jost Project Authors) is under the SIL Open Font License 1.1 (`static/fonts/Jost-OFL.txt`).
