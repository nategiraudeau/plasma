package plasma

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RGB is an 8-bit terminal color.
type RGB struct {
	R uint8
	G uint8
	B uint8
}

// Text returns only the character grid. It has no trailing newline, making it
// suitable for embedding in a larger terminal view.
func (f Frame) Text() string {
	grid := make([][]rune, f.Rows)
	for r := range grid {
		grid[r] = make([]rune, f.Columns)
		for c := range grid[r] {
			grid[r][c] = ' '
		}
	}
	for _, cell := range f.Cells {
		grid[cell.Row][cell.Column] = cell.Glyph
	}

	var b strings.Builder
	for r, row := range grid {
		if r != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(string(row))
	}
	return b.String()
}

// ANSI returns a true-color terminal rendering. Terminal protocols have no
// foreground alpha, so every glyph dynamically chooses the RGB color produced
// by alpha-compositing foreground over background.
func (f Frame) ANSI(foreground, background RGB) string {
	return f.ANSIAB(foreground, foreground, background)
}

// ANSIAB renders with an A/B color mix that follows each glyph's position in
// the density ramp, independent of its opacity. Canvas opacity continues to
// control only how strongly the glyph shows over the background.
func (f Frame) ANSIAB(a, colorB, background RGB) string {
	b := make([]byte, 0, f.Columns*f.Rows*2)
	cellIndex := 0
	for r := 0; r < f.Rows; r++ {
		if r != 0 {
			b = append(b, '\n')
		}
		// Paint the supplied background as well as compositing glyph colors
		// against it, so the frame is self-contained on any terminal theme.
		b = append(b, "\x1b[48;2;"...)
		b = strconv.AppendInt(b, int64(background.R), 10)
		b = append(b, ';')
		b = strconv.AppendInt(b, int64(background.G), 10)
		b = append(b, ';')
		b = strconv.AppendInt(b, int64(background.B), 10)
		b = append(b, 'm')
		lastR, lastG, lastB := -1, -1, -1
		for c := 0; c < f.Columns; c++ {
			if cellIndex >= len(f.Cells) || f.Cells[cellIndex].Row != r || f.Cells[cellIndex].Column != c {
				b = append(b, ' ')
				continue
			}
			cell := f.Cells[cellIndex]
			cellIndex++
			alpha := terminalAlpha(cell.Opacity)
			mix := densityMixRatio(cell.Glyph)
			source := interpolate(a, colorB, mix)
			r := composite(int(source.R), int(background.R), alpha)
			g := composite(int(source.G), int(background.G), alpha)
			bl := composite(int(source.B), int(background.B), alpha)
			if r != lastR || g != lastG || bl != lastB {
				b = append(b, "\x1b[38;2;"...)
				b = strconv.AppendInt(b, int64(r), 10)
				b = append(b, ';')
				b = strconv.AppendInt(b, int64(g), 10)
				b = append(b, ';')
				b = strconv.AppendInt(b, int64(bl), 10)
				b = append(b, 'm')
				lastR, lastG, lastB = r, g, bl
			}
			b = utf8.AppendRune(b, cell.Glyph)
		}
		b = append(b, "\x1b[0m"...)
	}
	return string(b)
}

// ANSIQuantized returns a full terminal view with a small foreground palette.
// It is intended for screen-diffing renderers such as Bubble Tea: glyphs remain
// exact, while stable shade bands let the renderer skip most cells between
// frames. Background is used for blending but is owned by the outer renderer.
func (f Frame) ANSIQuantized(foreground, background RGB, shades int) string {
	return NewQuantizedRenderer(shades).Render(f, foreground, background)
}

// ANSIQuantizedAB is the stateless convenience form of RenderAB.
func (f Frame) ANSIQuantizedAB(a, b, background RGB, shades int) string {
	return NewQuantizedRenderer(shades).RenderAB(f, a, b, background)
}

// QuantizedRenderer retains its palette and byte buffer between frames. Use
// one per animated view to avoid rebuilding color escapes and scratch storage.
type QuantizedRenderer struct {
	shades     int
	colorA     RGB
	colorB     RGB
	background RGB
	palette    [][][]byte // palette[densityIndex][shade]
	buffer     []byte
}

// NewQuantizedRenderer creates a renderer with between 2 and 256 alpha shades.
func NewQuantizedRenderer(shades int) *QuantizedRenderer {
	return &QuantizedRenderer{shades: clamp(shades, 2, 256)}
}

// Render returns an immutable full-frame string suitable for tea.View content.
// The returned string is the only required per-frame allocation.
func (r *QuantizedRenderer) Render(f Frame, foreground, background RGB) string {
	return r.RenderAB(f, foreground, foreground, background)
}

// RenderAB renders a full frame using the density-ramp A/B color mix.
func (r *QuantizedRenderer) RenderAB(f Frame, a, colorB, background RGB) string {
	if r.palette == nil || a != r.colorA || colorB != r.colorB || background != r.background {
		r.buildPalette(a, colorB, background)
	}

	output := r.buffer[:0]
	if cap(output) < f.Columns*f.Rows*2 {
		output = make([]byte, 0, f.Columns*f.Rows*2)
	}
	cellIndex, lastShade, lastDensity := 0, -1, -1
	for row := 0; row < f.Rows; row++ {
		if row > 0 {
			output = append(output, '\n')
		}
		for column := 0; column < f.Columns; column++ {
			if cellIndex >= len(f.Cells) || f.Cells[cellIndex].Row != row || f.Cells[cellIndex].Column != column {
				output = append(output, ' ')
				continue
			}
			cell := f.Cells[cellIndex]
			cellIndex++
			shade := clamp(int(math.Floor(cell.Opacity/maxOpacity*float64(r.shades-1)+0.5)), 0, r.shades-1)
			if shade == 0 {
				output = append(output, ' ')
				continue
			}
			density := densityIndexOf(cell.Glyph)
			if shade != lastShade || density != lastDensity {
				output = append(output, r.palette[density][shade]...)
				lastShade, lastDensity = shade, density
			}
			output = utf8.AppendRune(output, cell.Glyph)
		}
	}
	if lastShade >= 0 {
		output = append(output, "\x1b[39m"...)
	}
	r.buffer = output
	return string(output)
}

func (r *QuantizedRenderer) buildPalette(a, colorB, background RGB) {
	r.colorA, r.colorB, r.background = a, colorB, background
	n := len(densityRunes)
	if len(r.palette) != n {
		r.palette = make([][][]byte, n)
	}
	for density := 1; density < n; density++ {
		mix := densityMixRatio(densityRunes[density])
		source := interpolate(a, colorB, mix)
		if len(r.palette[density]) != r.shades {
			r.palette[density] = make([][]byte, r.shades)
		}
		for shade := 1; shade < r.shades; shade++ {
			alpha := float64(shade) / float64(r.shades-1)
			color := RGB{
				R: uint8(composite(int(source.R), int(background.R), alpha)),
				G: uint8(composite(int(source.G), int(background.G), alpha)),
				B: uint8(composite(int(source.B), int(background.B), alpha)),
			}
			r.palette[density][shade] = appendForeground(r.palette[density][shade][:0], color)
		}
	}
}

func appendForeground(dst []byte, color RGB) []byte {
	dst = append(dst, "\x1b[38;2;"...)
	dst = strconv.AppendInt(dst, int64(color.R), 10)
	dst = append(dst, ';')
	dst = strconv.AppendInt(dst, int64(color.G), 10)
	dst = append(dst, ';')
	dst = strconv.AppendInt(dst, int64(color.B), 10)
	return append(dst, 'm')
}

func composite(fg, bg int, alpha float64) int {
	return int(float64(fg)*alpha + float64(bg)*(1-alpha) + 0.5)
}

func interpolate(a, b RGB, alpha float64) RGB {
	return RGB{
		R: uint8(composite(int(a.R), int(b.R), alpha)),
		G: uint8(composite(int(a.G), int(b.G), alpha)),
		B: uint8(composite(int(a.B), int(b.B), alpha)),
	}
}

func terminalAlpha(canvasOpacity float64) float64 {
	return math.Max(0, math.Min(1, canvasOpacity/maxOpacity))
}

// densityIndexByGlyph maps each visible ramp glyph back to its position in
// DensityRamp, built once from the ramp itself so the two never drift apart.
var densityIndexByGlyph = buildDensityIndex()

func buildDensityIndex() map[rune]int {
	m := make(map[rune]int, len(densityRunes))
	for i, glyph := range densityRunes {
		m[glyph] = i
	}
	return m
}

func densityIndexOf(glyph rune) int {
	return densityIndexByGlyph[glyph]
}

// densityMixRatio maps a glyph's ramp position to a 0..1 A/B mix ratio,
// linear across the visible glyphs (everything but the leading space) so the
// color gradient spans the full ramp regardless of the opacity curve. The
// sparsest visible glyph is pure colorB; the densest is pure colorA.
func densityMixRatio(glyph rune) float64 {
	index := densityIndexOf(glyph)
	if index <= 1 {
		return 0
	}
	return float64(index-1) / float64(len(densityRunes)-2)
}
