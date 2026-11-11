package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Covers are kept at most coverMax pixels on a side and re-encoded at
// coverQuality. Discogs' API serves images of at most 600 px (all 319 stored
// covers measured 2026-11-11 were ≤ 600), and the largest place a cover is
// shown — a dug-into pile, 420 CSS px, and the record card — needs more than
// that on a 2× screen, so 600 is kept as it is and only larger images shrink.
// Re-encoding those covers at quality 75 with Go's encoder took them from
// 34.6 MB to 16.9 MB with no difference visible at 1:1 (measured on the Pi).
const (
	coverMax     = 600
	coverQuality = 75
	// coversOptimized marks a covers directory whose files have all been
	// through optimizeCover once.
	coversOptimized = ".optimized-1"
)

// optimizeCover returns the cover to store: shrunk to coverMax and
// re-encoded, or the original bytes when they cannot be decoded or the new
// file would not be smaller.
func optimizeCover(b []byte) []byte {
	img, err := jpeg.Decode(bytes.NewReader(b))
	if err != nil {
		return b
	}
	shrunk := false
	if r := img.Bounds(); r.Dx() > coverMax || r.Dy() > coverMax {
		img = shrink(img, coverMax)
		shrunk = true
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: coverQuality}); err != nil {
		return b
	}
	if !shrunk && buf.Len() >= len(b) {
		return b
	}
	return buf.Bytes()
}

// shrink scales img down so its longer side is side px, averaging every source
// pixel that falls into each new one (the standard library has no scaler).
func shrink(img image.Image, side int) image.Image {
	r := img.Bounds()
	w, h := r.Dx(), r.Dy()
	nw, nh := side, side
	if w > h {
		nh = (h*side + w/2) / w
	} else {
		nw = (w*side + h/2) / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := r.Min.Y+y*h/nh, r.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := r.Min.X+x*w/nw, r.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sr, sg, sb, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, _ := img.At(xx, yy).RGBA()
					sr, sg, sb = sr+uint64(cr), sg+uint64(cg), sb+uint64(cb)
					n++
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(sr / n >> 8), uint8(sg / n >> 8), uint8(sb / n >> 8), 255})
		}
	}
	return out
}

// optimizeStoredCovers runs optimizeCover once over covers downloaded before
// it existed. Their URLs change by themselves (coverURL includes the file
// size), so browsers fetch the smaller files.
func (a *App) optimizeStoredCovers() {
	marker := filepath.Join(a.coversDir, coversOptimized)
	if _, err := os.Stat(marker); err == nil {
		return
	}
	files, err := filepath.Glob(filepath.Join(a.coversDir, "*.jpg"))
	if err != nil {
		return
	}
	var before, after int64
	changed := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		nb := optimizeCover(b)
		before += int64(len(b))
		after += int64(len(nb))
		if len(nb) == len(b) {
			continue
		}
		tmp := strings.TrimSuffix(f, ".jpg") + ".opt.tmp"
		if os.WriteFile(tmp, nb, 0o644) != nil || os.Rename(tmp, f) != nil {
			os.Remove(tmp)
			continue
		}
		changed++
	}
	log.Printf("covers: optimized %d of %d, %.1f MB → %.1f MB", changed, len(files), float64(before)/1e6, float64(after)/1e6)
	os.WriteFile(marker, []byte("ok\n"), 0o644)
}
