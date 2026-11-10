# vinyl-catalogue

A read-only showcase of one Discogs user's record collection. One Go binary built only on the standard library, all state in one JSON file plus a folder of covers, runs in Docker on a Raspberry Pi behind a Cloudflare Tunnel.

There is no admin panel, no accounts and nothing social: the collection is edited on Discogs, and the site reads it from the Discogs API on its own.

The site is in Russian and English (picked from the browser, with a switch), with a day and a night theme.

## Features

- **Stacks.** The collection stands in piles on a horizontal rail, like crates in a record shop: by artist initial (default), genre, decade, or year of the original release. Each pile shows the cover of its first record and, behind it, the tops of the other sleeves — the nearest one exactly as wide as the cover, the pile narrowing by at most 1% a side toward the back; a pile of one record has none.
- **Crate digging.** The pile in the middle is entered explicitly (Enter or a click) and then dug through in place: the front record tilts forward and falls toward you, uncovering the next one. The records already flipped lie flat in front of the pile, below the front record and never over its cover, drawn as plain edges (a face-down cover would not show) — so the fallen pile grows as the pile behind shrinks; before the first flip there is none. Every pile has the same height and a fixed label box, so the row never moves when a long artist or title wraps. The label under the pile shows artist, title and "3 of 15". The piles on either side stay piles.

  | | Keyboard | Mouse, trackpad | Touch |
  |---|---|---|---|
  | Next / previous pile | → / ← | arrow buttons, a click on a side pile, the index under the rail, sideways scrolling; the wheel in the overview | swipe sideways |
  | Start digging the middle pile | Enter | a click on it | tap |
  | Next / previous record (while digging) | ↓ / ↑ | the ⌄ / ⌃ buttons under the pile, the wheel | swipe up / down |
  | Open the front record (while digging) | Enter | a click on it | tap |
  | Close the record, then stop digging | Esc, Esc | ✕ or a click outside; **Back to the collection** | the same |

  The wheel and two-finger trackpad swipes follow the hand: down means the next record, like ↓ (on Apple devices, where scrolling is "natural" by default, the browser reports a downward swipe as scrolling up, so the direction is turned there; a Mac with natural scrolling switched off gets it reversed — the browser cannot see that setting). One swipe, with its inertia, moves one step. A new swipe counts at once, even while the previous one's inertia is still arriving: the inertia only slows down, so two events in a row that speed up again (after it slowed to 40% of its peak, and back to at least half the peak) mark a new swipe, as does a change of direction or 150 ms without wheel events; two steps are at least 140 ms apart.

  The address keeps the place (`?s=J&r=2` is the third record of pile J), so a reload or a shared link opens the same record at the front.
- **Wall.** Every cover in a grid, under the same grouping.
- **Normal or reverse order** for any grouping, in stacks and on the wall: the button reverses both the order of the stacks and the records in each one (Z…A, newest first), keeps the same stack in the middle, and is kept in the address (`?o=desc`).
- **Order.** Artists are filed the way record shops do it: case and accents are ignored, and a leading "The" is dropped (The Beatles stand under B). Records of one artist go by the year of the original release (the Discogs master), then by title. Latin initials come first, then Cyrillic, then everything else under "#".
- **Search** over artist, title, genres and styles, labels, catalogue numbers, format and years; every word must match.
- **Random** opens a random record (from the search results, if there is a search).
- **Release card** (an overlay on the collection page, and a page of its own at `/release/<id>/<slug>` for direct links): cover, artist, title, original year, genres and styles, labels and catalogue numbers, format as Discogs writes it ("1 × Vinyl, LP, Album, Reissue, Orange"), this pressing's release date and country, the tracklist with headings and durations, the Discogs release notes (folded), and a link to the release on Discogs.
- **Nothing personal.** The site keeps and shows only what Discogs says about the releases: no date added, no rating, no condition, no notes, no profile.
- **Themes.** The theme follows the system until the visitor picks one with the button in the header (system → day → night); the choice is kept in the browser.
- **Link previews**, always in English: the collection page shares a 1200×630 card (`static/og.png`: "My record collection — Dig through the crates", the domain, a record sliding out of its sleeve) with the owner's name and the number of releases in the text; a record's page shares its own cover, title, original year and format. Full Open Graph and Twitter tags, plus `favicon.ico`, `favicon-32.png`, `icon.svg` and a 180×180 `apple-touch-icon.png` for Safari Favorites and the iOS home screen.
- `noindex` for search engines; link-preview crawlers (Twitter, Facebook, Telegram, Slack, Discord, WhatsApp, LinkedIn, Bluesky's Cardyb) may read the pages.

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
| `app.go` | Routes, security headers, rendering, covers, language switch |
| `i18n.go` | Russian and English texts, Russian plural forms |
| `templates/` | Page layout, collection page, release card |
| `static/app.js` | Stacks and crate digging, wall, search, grouping, overlay, keyboard, wheel and touch |
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
| `DISCOGS_TOKEN` | Optional personal access token; only raises the API limit |
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

## License

No license has been chosen yet.
