package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Artist struct {
	Name string `json:"name"` // as credited (ANV), else the Discogs name without " (2)"
	Sort string `json:"sort"` // Discogs name without " (2)", for ordering
	Join string `json:"join,omitempty"`
	ID   int64  `json:"id,omitempty"`
}

type Format struct {
	Name         string   `json:"name"`
	Qty          string   `json:"qty,omitempty"`
	Text         string   `json:"text,omitempty"`
	Descriptions []string `json:"descriptions,omitempty"`
}

type Label struct {
	Name  string `json:"name"`
	Catno string `json:"catno,omitempty"`
}

type Track struct {
	Position string  `json:"position,omitempty"`
	Heading  bool    `json:"heading,omitempty"`
	Title    string  `json:"title"`
	Duration string  `json:"duration,omitempty"`
	Artists  string  `json:"artists,omitempty"` // track artists on compilations
	Sub      []Track `json:"sub,omitempty"`
}

// Details come from /releases/{id}, one request per release, so they are
// read once and kept.
type Details struct {
	Country   string  `json:"country,omitempty"`
	Released  string  `json:"released,omitempty"` // "2023-09-15", "1985-00-00" or "1985"
	Notes     string  `json:"notes,omitempty"`
	Tracklist []Track `json:"tracklist,omitempty"`
	// Cover is the release's primary image on Discogs. It wins over the
	// collection list's cover_image, so a better scan uploaded to Discogs
	// replaces ours at the next daily refresh.
	Cover  string    `json:"cover,omitempty"`
	ReadAt time.Time `json:"read_at"`
}

// Release is one copy in the collection (a collection instance). The same
// Discogs release owned twice is two Releases with one ID. Nothing personal
// (date added, rating, condition, notes) is kept.
type Release struct {
	Instance int64    `json:"instance"`
	ID       int64    `json:"id"`
	MasterID int64    `json:"master_id,omitempty"`
	Title    string   `json:"title"`
	Artists  []Artist `json:"artists"`
	Year     int      `json:"year,omitempty"` // this pressing
	Genres   []string `json:"genres,omitempty"`
	Styles   []string `json:"styles,omitempty"`
	Formats  []Format `json:"formats,omitempty"`
	Labels   []Label  `json:"labels,omitempty"`
	CoverURL string   `json:"cover_url,omitempty"` // cover_image from the collection list
}

type Owner struct {
	Username string `json:"username"`
	Name     string `json:"name,omitempty"`
}

// Data is the whole state, kept in DATA_DIR/catalogue.json.
type Data struct {
	Owner    Owner     `json:"owner"`
	Releases []Release `json:"releases"` // in the order Discogs lists them (newest first)
	// Details and MasterYears are keyed by Discogs id and outlive the
	// releases that needed them, so a record removed and added back costs no
	// new requests.
	Details     map[int64]*Details `json:"details"`
	MasterYears map[int64]int      `json:"master_years"`
	SyncedAt    time.Time          `json:"synced_at"`
	// FullSyncAt is the last sync that re-read every release (details and
	// cover), which happens once per SYNC_INTERVAL.
	FullSyncAt time.Time `json:"full_sync_at"`
}

type Store struct {
	path string
	mu   sync.RWMutex
	data Data
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "catalogue.json")}
	b, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &s.data); err != nil {
			return nil, err
		}
	}
	if s.data.Details == nil {
		s.data.Details = map[int64]*Details{}
	}
	if s.data.MasterYears == nil {
		s.data.MasterYears = map[int64]int{}
	}
	return s, nil
}

func (s *Store) view(f func(d *Data)) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f(&s.data)
}

// update changes the data under the lock and writes it to disk.
func (s *Store) update(f func(d *Data)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.data)
	return s.save()
}

// save writes the file atomically: temp file, fsync, rename.
func (s *Store) save() error {
	b, err := json.Marshal(&s.data)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "catalogue-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
