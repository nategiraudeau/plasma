package plasma

import (
	"math"
	"strconv"
	"unicode/utf8"
)

const maxOpacity = 0.55

// ScreenRenderer efficiently presents plasma frames in a terminal it owns.
// It quantizes the simulated alpha into a small palette, retains the previous
// screen, and emits only changed runs. Glyph selection is never quantized.
type ScreenRenderer struct {
	columns int
	rows    int
	shades  int

	previous          []uint32
	current           []uint32
	palette           [][][]byte // palette[densityIndex][shade]
	bgSGR             []byte
	colorA            RGB
	colorB            RGB
	bg                RGB
	defaultBackground bool

	initialized bool
	lastShade   int
	lastDensity int
}

// NewScreenRenderer creates a delta renderer. Twenty-four shades is usually
// visually indistinguishable from continuous opacity while greatly reducing
// terminal traffic. Values below two are promoted to two and above 256 capped.
func NewScreenRenderer(columns, rows, shades int) *ScreenRenderer {
	r := &ScreenRenderer{shades: clamp(shades, 2, 256), lastShade: -1, lastDensity: -1}
	r.Resize(columns, rows)
	return r
}

// Resize invalidates the retained screen and forces the next update to repaint.
func (r *ScreenRenderer) Resize(columns, rows int) {
	r.columns = max(columns, 0)
	r.rows = max(rows, 0)
	size := r.columns * r.rows
	if cap(r.previous) < size {
		r.previous = make([]uint32, size)
		r.current = make([]uint32, size)
	} else {
		r.previous = r.previous[:size]
		r.current = r.current[:size]
		clear(r.previous)
		clear(r.current)
	}
	r.initialized = false
	r.lastShade = -1
	r.lastDensity = -1
}

// AppendFrame appends the minimal ANSI update needed to display frame. The
// returned bytes use DEC synchronized updates to avoid partial-frame tearing;
// terminals that do not implement the mode safely ignore it.
func (r *ScreenRenderer) AppendFrame(dst []byte, frame Frame, foreground, background RGB) []byte {
	return r.AppendFrameAB(dst, frame, foreground, foreground, background)
}

// AppendFrameAB appends a delta using the density-ramp A/B color mix.
func (r *ScreenRenderer) AppendFrameAB(dst []byte, frame Frame, a, colorB, background RGB) []byte {
	return r.appendFrame(dst, frame, a, colorB, background, false)
}

// AppendFrameDefaultBackground is AppendFrame without painting an explicit
// background color. It uses the terminal's default background while using
// estimatedBackground only to calculate simulated foreground opacity. This is
// the correct fallback when an OSC 11 query is unavailable.
func (r *ScreenRenderer) AppendFrameDefaultBackground(dst []byte, frame Frame, foreground, estimatedBackground RGB) []byte {
	return r.appendFrame(dst, frame, foreground, foreground, estimatedBackground, true)
}

// AppendFrameDefaultBackgroundAB combines the A/B fade with the terminal's
// default background, using estimatedBackground only for color calculations.
func (r *ScreenRenderer) AppendFrameDefaultBackgroundAB(dst []byte, frame Frame, a, colorB, estimatedBackground RGB) []byte {
	return r.appendFrame(dst, frame, a, colorB, estimatedBackground, true)
}

func (r *ScreenRenderer) appendFrame(dst []byte, frame Frame, a, colorB, background RGB, defaultBackground bool) []byte {
	if frame.Columns != r.columns || frame.Rows != r.rows {
		r.Resize(frame.Columns, frame.Rows)
	}
	if a != r.colorA || colorB != r.colorB || background != r.bg || defaultBackground != r.defaultBackground || r.palette == nil {
		r.buildPalette(a, colorB, background, defaultBackground)
		r.initialized = false
	}

	clear(r.current)
	for _, cell := range frame.Cells {
		shade := int(math.Floor(cell.Opacity/maxOpacity*float64(r.shades-1) + 0.5))
		shade = clamp(shade, 0, r.shades-1)
		if shade == 0 {
			continue
		}
		density := densityIndexOf(cell.Glyph)
		r.current[cell.Row*r.columns+cell.Column] = uint32(cell.Glyph)<<12 | uint32(density)<<8 | uint32(shade)
	}

	bodyStart := len(dst)
	dst = append(dst, "\x1b[?2026h"...)
	full := !r.initialized
	if full {
		dst = append(dst, r.bgSGR...)
		dst = append(dst, "\x1b[2J"...)
		r.lastShade = -1
		r.lastDensity = -1
	}

	for row := 0; row < r.rows; row++ {
		base := row * r.columns
		wroteRow := false
		for column := 0; column < r.columns; {
			if !r.needsWrite(base+column, full) {
				column++
				continue
			}

			start := column
			end := column + 1
			// Merge changes separated by at most two stable cells. Rewriting two
			// cells is cheaper than another absolute cursor-position sequence.
			for end < r.columns {
				if r.needsWrite(base+end, full) {
					end++
					continue
				}
				gapEnd := end
				for gapEnd < r.columns && gapEnd-end < 3 && !r.needsWrite(base+gapEnd, full) {
					gapEnd++
				}
				if gapEnd < r.columns && r.needsWrite(base+gapEnd, full) {
					end = gapEnd + 1
					continue
				}
				break
			}

			if wroteRow {
				dst = appendColumn(dst, start+1)
			} else {
				dst = appendPosition(dst, row+1, start+1)
				wroteRow = true
			}
			for position := base + start; position < base+end; position++ {
				packed := r.current[position]
				if packed == 0 {
					dst = append(dst, ' ')
					continue
				}
				shade := int(packed & 0xff)
				density := int((packed >> 8) & 0xf)
				if shade != r.lastShade || density != r.lastDensity {
					dst = append(dst, r.palette[density][shade]...)
					r.lastShade, r.lastDensity = shade, density
				}
				dst = utf8.AppendRune(dst, rune(packed>>12))
			}
			column = end
		}
	}

	if len(dst) == bodyStart+len("\x1b[?2026h") {
		// No cells changed, so do not write synchronization controls either.
		dst = dst[:bodyStart]
	} else {
		dst = append(dst, "\x1b[?2026l"...)
	}
	copy(r.previous, r.current)
	r.initialized = true
	return dst
}

func (r *ScreenRenderer) needsWrite(position int, full bool) bool {
	if full {
		return r.current[position] != 0
	}
	return r.current[position] != r.previous[position]
}

func (r *ScreenRenderer) buildPalette(a, colorB, background RGB, defaultBackground bool) {
	r.colorA, r.colorB, r.bg, r.defaultBackground = a, colorB, background, defaultBackground
	n := len(densityRunes)
	r.palette = make([][][]byte, n)
	for density := 1; density < n; density++ {
		mix := densityMixRatio(densityRunes[density])
		source := interpolate(a, colorB, mix)
		r.palette[density] = make([][]byte, r.shades)
		for shade := 1; shade < r.shades; shade++ {
			alpha := float64(shade) / float64(r.shades-1)
			red := composite(int(source.R), int(background.R), alpha)
			green := composite(int(source.G), int(background.G), alpha)
			blue := composite(int(source.B), int(background.B), alpha)
			sequence := make([]byte, 0, 24)
			sequence = append(sequence, "\x1b[38;2;"...)
			sequence = strconv.AppendInt(sequence, int64(red), 10)
			sequence = append(sequence, ';')
			sequence = strconv.AppendInt(sequence, int64(green), 10)
			sequence = append(sequence, ';')
			sequence = strconv.AppendInt(sequence, int64(blue), 10)
			sequence = append(sequence, 'm')
			r.palette[density][shade] = sequence
		}
	}
	r.bgSGR = r.bgSGR[:0]
	if defaultBackground {
		r.bgSGR = append(r.bgSGR, "\x1b[49m"...)
		return
	}
	r.bgSGR = append(r.bgSGR, "\x1b[48;2;"...)
	r.bgSGR = strconv.AppendInt(r.bgSGR, int64(background.R), 10)
	r.bgSGR = append(r.bgSGR, ';')
	r.bgSGR = strconv.AppendInt(r.bgSGR, int64(background.G), 10)
	r.bgSGR = append(r.bgSGR, ';')
	r.bgSGR = strconv.AppendInt(r.bgSGR, int64(background.B), 10)
	r.bgSGR = append(r.bgSGR, 'm')
}

func appendPosition(dst []byte, row, column int) []byte {
	dst = append(dst, '\x1b', '[')
	dst = strconv.AppendInt(dst, int64(row), 10)
	dst = append(dst, ';')
	dst = strconv.AppendInt(dst, int64(column), 10)
	return append(dst, 'H')
}

func appendColumn(dst []byte, column int) []byte {
	dst = append(dst, '\x1b', '[')
	dst = strconv.AppendInt(dst, int64(column), 10)
	return append(dst, 'G')
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
