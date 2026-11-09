package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Listen  string
	DataDir string
	// DiscogsUser is whose collection is shown.
	DiscogsUser string
	// DiscogsToken is an optional personal access token. It raises the API
	// limit from 25 to 60 requests a minute; nothing else depends on it.
	DiscogsToken string
	// SyncEvery is how often every release is read again (details and
	// cover); a day by default.
	SyncEvery time.Duration
	// BaseURL is the public origin, used for absolute links in link previews.
	BaseURL string
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func loadConfig() (Config, error) {
	c := Config{
		Listen:       env("LISTEN_ADDR", ":8097"),
		DataDir:      env("DATA_DIR", "./data"),
		DiscogsUser:  env("DISCOGS_USER", ""),
		DiscogsToken: env("DISCOGS_TOKEN", ""),
		BaseURL:      strings.TrimRight(env("BASE_URL", ""), "/"),
	}
	if c.DiscogsUser == "" {
		return c, errors.New("DISCOGS_USER is not set")
	}
	d, err := time.ParseDuration(env("SYNC_INTERVAL", "24h"))
	if err != nil || d < time.Hour {
		return c, fmt.Errorf("SYNC_INTERVAL must be a duration of at least 1h, e.g. 24h")
	}
	c.SyncEvery = d
	return c, nil
}
