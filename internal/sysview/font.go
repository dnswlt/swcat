package sysview

import (
	"bytes"
	"compress/zlib"
	"embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"
)

// Noto Sans is the font the graph SVGs are rendered with in the browser (see
// web/main.js and the .graphviz-svg rule in web/style.css). Layout has to size
// boxes to fit their text, so it needs the very same glyph advances the browser
// will use. Reading a font from the host would make layout depend on what is
// installed on the machine (dot's long-standing weakness here), so we embed the
// exact same subset files the frontend loads and measure against those.
//
//go:embed fonts/*.woff
var fontFS embed.FS

var fontFiles = []string{
	"fonts/noto-sans-latin-400-normal.woff",
	"fonts/noto-sans-latin-ext-400-normal.woff",
}

// fallbackAdvance is used for runes outside the embedded subsets. Those render
// in a fallback font in the browser anyway, so any value is an estimate; we pick
// a generous one so boxes end up too wide rather than too narrow.
const fallbackAdvance = 0.62

var (
	metricsOnce sync.Once
	// advances maps a rune to its advance width in em units (1.0 == font size).
	advances map[rune]float64
)

func fontMetrics() map[rune]float64 {
	metricsOnce.Do(func() {
		advances = make(map[rune]float64, 1024)
		for _, name := range fontFiles {
			data, err := fontFS.ReadFile(name)
			if err != nil {
				log.Printf("sysview: cannot read embedded font %s: %v", name, err)
				continue
			}
			m, err := parseWOFFAdvances(data)
			if err != nil {
				log.Printf("sysview: cannot parse embedded font %s: %v", name, err)
				continue
			}
			for r, a := range m {
				// Subsets overlap (both contain ASCII); identical values, so
				// first one wins is fine.
				if _, ok := advances[r]; !ok {
					advances[r] = a
				}
			}
		}
	})
	return advances
}

// TextWidth returns the rendered width of s at the given font size, in the same
// units as the font size. Advances are summed without kerning, which slightly
// overestimates for kerned pairs — the safe direction for box fitting.
func TextWidth(s string, fontSize float64) float64 {
	m := fontMetrics()
	var w float64
	for _, r := range s {
		if a, ok := m[r]; ok {
			w += a
		} else {
			w += fallbackAdvance
		}
	}
	return w * fontSize
}

// --- WOFF 1.0 parsing -------------------------------------------------------
//
// A WOFF file is a thin container around the sfnt tables of a TrueType font:
// a 44-byte header, a table directory, and the tables themselves, each either
// stored verbatim or zlib-compressed. Only four tables are needed to measure
// text: head (units per em), hhea (number of h-metrics), hmtx (the advances),
// and cmap (rune to glyph id).

func parseWOFFAdvances(data []byte) (map[rune]float64, error) {
	tables, err := woffTables(data)
	if err != nil {
		return nil, err
	}

	head, ok := tables["head"]
	if !ok || len(head) < 54 {
		return nil, fmt.Errorf("missing or short head table")
	}
	unitsPerEm := float64(be16(head, 18))
	if unitsPerEm == 0 {
		return nil, fmt.Errorf("invalid unitsPerEm")
	}

	hhea, ok := tables["hhea"]
	if !ok || len(hhea) < 36 {
		return nil, fmt.Errorf("missing or short hhea table")
	}
	numHMetrics := int(be16(hhea, 34))

	hmtx, ok := tables["hmtx"]
	if !ok {
		return nil, fmt.Errorf("missing hmtx table")
	}
	cmap, ok := tables["cmap"]
	if !ok {
		return nil, fmt.Errorf("missing cmap table")
	}

	glyphAdvance := func(gid int) (float64, bool) {
		if gid < 0 || numHMetrics == 0 {
			return 0, false
		}
		// Glyphs beyond numHMetrics all share the last advance (monospaced tail).
		i := gid
		if i >= numHMetrics {
			i = numHMetrics - 1
		}
		off := i * 4
		if off+2 > len(hmtx) {
			return 0, false
		}
		return float64(be16(hmtx, off)) / unitsPerEm, true
	}

	runeToGID, err := parseCmap(cmap)
	if err != nil {
		return nil, err
	}

	out := make(map[rune]float64, len(runeToGID))
	for r, gid := range runeToGID {
		if a, ok := glyphAdvance(gid); ok {
			out[r] = a
		}
	}
	return out, nil
}

// woffTables decompresses a WOFF file and returns its sfnt tables by tag.
func woffTables(data []byte) (map[string][]byte, error) {
	const woffHeaderSize = 44
	const dirEntrySize = 20
	if len(data) < woffHeaderSize || string(data[0:4]) != "wOFF" {
		return nil, fmt.Errorf("not a WOFF file")
	}
	numTables := int(be16(data, 12))
	tables := make(map[string][]byte, numTables)
	for i := range numTables {
		off := woffHeaderSize + i*dirEntrySize
		if off+dirEntrySize > len(data) {
			return nil, fmt.Errorf("truncated table directory")
		}
		tag := string(data[off : off+4])
		tblOff := int(be32(data, off+4))
		compLen := int(be32(data, off+8))
		origLen := int(be32(data, off+12))
		if tblOff < 0 || compLen < 0 || tblOff+compLen > len(data) {
			return nil, fmt.Errorf("table %q out of bounds", tag)
		}
		raw := data[tblOff : tblOff+compLen]
		if compLen == origLen {
			// Stored verbatim.
			tables[tag] = raw
			continue
		}
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("table %q: %w", tag, err)
		}
		out, err := io.ReadAll(zr)
		zr.Close()
		if err != nil {
			return nil, fmt.Errorf("table %q: %w", tag, err)
		}
		tables[tag] = out
	}
	return tables, nil
}

// parseCmap returns a rune -> glyph id mapping from a cmap table, preferring a
// Unicode subtable in format 12 (full range) or 4 (BMP).
func parseCmap(cmap []byte) (map[rune]int, error) {
	if len(cmap) < 4 {
		return nil, fmt.Errorf("short cmap table")
	}
	numSubtables := int(be16(cmap, 2))
	var best []byte
	var bestFormat int
	for i := range numSubtables {
		off := 4 + i*8
		if off+8 > len(cmap) {
			break
		}
		platformID := be16(cmap, off)
		encodingID := be16(cmap, off+2)
		subOff := int(be32(cmap, off+4))
		if subOff+4 > len(cmap) {
			continue
		}
		unicode := platformID == 0 ||
			(platformID == 3 && (encodingID == 1 || encodingID == 10))
		if !unicode {
			continue
		}
		format := int(be16(cmap, subOff))
		// Prefer format 12 over 4; ignore everything else.
		if (format == 12 || format == 4) && format > bestFormat {
			bestFormat, best = format, cmap[subOff:]
		}
	}
	switch bestFormat {
	case 12:
		return parseCmapFormat12(best)
	case 4:
		return parseCmapFormat4(best)
	default:
		return nil, fmt.Errorf("no usable cmap subtable")
	}
}

func parseCmapFormat4(t []byte) (map[rune]int, error) {
	if len(t) < 14 {
		return nil, fmt.Errorf("short cmap format 4 subtable")
	}
	segCount := int(be16(t, 6)) / 2
	endOff := 14
	startOff := endOff + segCount*2 + 2 // +2 for reservedPad
	deltaOff := startOff + segCount*2
	rangeOff := deltaOff + segCount*2
	if rangeOff+segCount*2 > len(t) {
		return nil, fmt.Errorf("truncated cmap format 4 subtable")
	}

	out := make(map[rune]int, 512)
	for seg := range segCount {
		end := rune(be16(t, endOff+seg*2))
		start := rune(be16(t, startOff+seg*2))
		delta := int(int16(be16(t, deltaOff+seg*2)))
		idRangeOffset := int(be16(t, rangeOff+seg*2))
		if start > end || end == 0xFFFF && start == 0xFFFF {
			continue
		}
		for c := start; c <= end; c++ {
			var gid int
			if idRangeOffset == 0 {
				gid = (int(c) + delta) & 0xFFFF
			} else {
				// Per the spec the offset is relative to the position of the
				// idRangeOffset entry itself.
				gi := rangeOff + seg*2 + idRangeOffset + int(c-start)*2
				if gi+2 > len(t) {
					continue
				}
				gid = int(be16(t, gi))
				if gid == 0 {
					continue
				}
				gid = (gid + delta) & 0xFFFF
			}
			if gid != 0 {
				out[c] = gid
			}
		}
	}
	return out, nil
}

func parseCmapFormat12(t []byte) (map[rune]int, error) {
	if len(t) < 16 {
		return nil, fmt.Errorf("short cmap format 12 subtable")
	}
	numGroups := int(be32(t, 12))
	out := make(map[rune]int, 512)
	for g := range numGroups {
		off := 16 + g*12
		if off+12 > len(t) {
			break
		}
		start := rune(be32(t, off))
		end := rune(be32(t, off+4))
		startGID := int(be32(t, off+8))
		if start > end || end-start > 0xFFFF {
			continue
		}
		for c := start; c <= end; c++ {
			out[c] = startGID + int(c-start)
		}
	}
	return out, nil
}

func be16(b []byte, off int) uint16 { return binary.BigEndian.Uint16(b[off : off+2]) }
func be32(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off : off+4]) }
