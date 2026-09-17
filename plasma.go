// Package plasma renders an animated ASCII plasma field with configurable masks.
package plasma

import (
	"fmt"
	"math"
	"strconv"
)

const (
	// FontSize is both the font size and row height in CSS pixels on the site.
	FontSize = 14.0
	// ColumnWidth is the site's assumed monospace character width.
	ColumnWidth = FontSize * 0.62
	// FrameStep advances each frame 30% faster than the original animation.
	FrameStep = 0.016 * 1.3
)

// DensityRamp orders glyphs from empty to full.
const DensityRamp = " ·.:;+|*#%@█"

var densityRunes = []rune(DensityRamp)

// Cell is one canvas fillText operation. Empty or nearly transparent cells are
// omitted from Frame.Cells, just as they are on the source canvas.
type Cell struct {
	Column   int
	Row      int
	X        float64 // canvas x coordinate in CSS pixels
	Baseline float64 // canvas text baseline in CSS pixels
	Glyph    rune
	Opacity  float64
	Alpha    string // Opacity formatted like JavaScript's op.toFixed(3)
}

// Frame is the deterministic output for one instant of the animation.
type Frame struct {
	Columns int
	Rows    int
	Time    float64
	Cells   []Cell
}

// Component is a reusable, deterministic animation state. Render does not
// mutate it; Step advances by a fixed amount per frame.
type Component struct {
	columns  int
	rows     int
	time     float64
	mask     Mask
	rotation float64

	xPhase                       []float64
	yPhase                       []float64
	xyPhase                      []float64
	distancePhase                []float64
	maskDistance                 []float64
	rotatedXPhase, rotatedYPhase []float64
}

// New creates a plasma component measured in terminal cells.
func New(columns, rows int) *Component {
	p := &Component{mask: DefaultMask()}
	p.Resize(columns, rows)
	return p
}

// NewFromPixels applies the exact grid sizing used by the source canvas.
func NewFromPixels(width, height int) *Component {
	return New(
		int(math.Floor(float64(width)/ColumnWidth)),
		int(math.Floor(float64(height)/FontSize)),
	)
}

// Resize changes the grid while preserving animation time.
func (p *Component) Resize(columns, rows int) {
	if columns < 0 {
		columns = 0
	}
	if rows < 0 {
		rows = 0
	}
	p.columns, p.rows = columns, rows
	p.rebuildGeometry()
	p.rebuildMaskGeometry()
}

// SetRotation rotates the plasma wave field clockwise about the
// region center, before glyph selection. Degrees must be finite; angles wrap
// into [0,360). Zero preserves the original output. Rotation uses ColumnWidth
// and FontSize to preserve proportions, and does not reset animation time.
// The mask, its feather, and its animated distortion stay in screen coordinates.
func (p *Component) SetRotation(degrees float64) error {
	if !finite(degrees) {
		return fmt.Errorf("rotation must be finite")
	}
	degrees = math.Mod(degrees, 360)
	if degrees < 0 {
		degrees += 360
	}
	if degrees == p.rotation {
		return nil
	}
	p.rotation = degrees
	p.rebuildGeometry()
	return nil
}

// Rotation returns the clockwise angle in degrees, normalized into [0,360).
func (p *Component) Rotation() float64 { return p.rotation }

// SetTime makes deterministic seeking and testing possible.
func (p *Component) SetTime(t float64) { p.time = t }

// Time reports the time value that the next Render call uses.
func (p *Component) Time() float64 { return p.time }

// Step advances one animation frame. Like the website, this is deliberately a
// fixed step rather than elapsed wall-clock time.
func (p *Component) Step() { p.time += FrameStep }

// Render computes the masked plasma glyphs and alpha values.
func (p *Component) Render() Frame {
	return p.RenderInto(nil)
}

// RenderInto computes a frame while reusing the supplied cell storage. A
// full-screen animation should retain Frame.Cells and pass it to the next call
// to avoid one allocation per frame.
func (p *Component) RenderInto(cells []Cell) Frame {
	return p.renderInto(cells, true)
}

// RenderFastInto is RenderInto without formatting Cell.Alpha strings. Terminal
// renderers only need the numeric opacity and should use this zero-allocation
// variant. All glyph and opacity calculations remain identical.
func (p *Component) RenderFastInto(cells []Cell) Frame {
	return p.renderInto(cells, false)
}

func (p *Component) renderInto(cells []Cell, formatAlpha bool) Frame {
	f := Frame{Columns: p.columns, Rows: p.rows, Time: p.time, Cells: cells[:0]}
	if p.columns == 0 || p.rows == 0 {
		return f
	}

	for r := 0; r < p.rows; r++ {
		// The default band has independent edge motion and no vertical fade.
		// Custom paths use the same normalized component coordinates.
		y := (float64(r) + 0.5) / float64(p.rows)
		left, right := p.mask.edges(y, p.time)
		for c := 0; c < p.columns; c++ {
			x := (float64(c) + 0.5) / float64(p.columns)
			position := r*p.columns + c
			xPhase, yPhase := p.xPhase[c], p.yPhase[r]
			if p.rotation != 0 {
				xPhase, yPhase = p.rotatedXPhase[position], p.rotatedYPhase[position]
			}
			mask := 0.0
			if p.mask.Path == nil {
				mask = p.mask.value(x, y, p.time, left, right)
			} else {
				mask = p.mask.pathValue(p.maskDistance[position], x, y, p.time)
			}
			if mask == 0 {
				continue
			}
			v := (math.Sin(xPhase+p.time) +
				math.Sin(yPhase+p.time*0.7) +
				math.Sin(p.xyPhase[position]+p.time*0.9) +
				math.Sin(p.distancePhase[position]+p.time*1.1)) / 4
			norm := (v + 1) / 2
			// Fading density also moves the ramp-derived A/B color toward B;
			// fading opacity then dissolves both colors into the background.
			idx := int(math.Floor(norm * mask * float64(len(densityRunes)-1)))
			// The formula is bounded, but guard against a one-ulp excursion.
			if idx < 0 {
				idx = 0
			} else if idx >= len(densityRunes) {
				idx = len(densityRunes) - 1
			}
			glyph := densityRunes[idx]
			if glyph == ' ' {
				continue
			}

			intensity := math.Sin(norm * math.Pi)
			opacity := intensity * 0.55 * mask
			if opacity < 0.006 {
				continue
			}

			cell := Cell{
				Column: c, Row: r,
				X: float64(c) * ColumnWidth, Baseline: float64(r+1) * FontSize,
				Glyph: glyph, Opacity: opacity,
			}
			if formatAlpha {
				cell.Alpha = fixed3(opacity)
			}
			f.Cells = append(f.Cells, cell)
		}
	}
	return f
}

// Edge displacement stays within 1.2% of screen width on either side.
func maskEdges(y, t float64) (left, right float64) {
	left = 0.42 + 0.008*math.Sin(y*5.0+t*0.23) + 0.004*math.Sin(y*9.0-t*0.17)
	// Finish the right fade beyond the viewport so it is clipped on screen.
	right = 1.07 + 0.008*math.Sin(y*4.3-t*0.19+1.7) + 0.004*math.Sin(y*7.0+t*0.13)
	return
}

func horizontalMask(x, left, right float64) float64 {
	// Broad S-curve shoulders ease into both the background and the core.
	const feather = 0.24
	return smootherstep((x-left)/feather) * smootherstep((right-x)/feather)
}

// Quintic easing has zero first and second derivatives at both ends.
func smootherstep(x float64) float64 {
	x = math.Max(0, math.Min(1, x))
	return x * x * x * (x*(x*6-15) + 10)
}

func (p *Component) rebuildGeometry() {
	p.xPhase = resizeFloat64(p.xPhase, p.columns)
	p.yPhase = resizeFloat64(p.yPhase, p.rows)
	size := p.columns * p.rows
	p.xyPhase = resizeFloat64(p.xyPhase, size)
	p.distancePhase = resizeFloat64(p.distancePhase, size)
	if p.rotation != 0 {
		p.rotatedXPhase = resizeFloat64(p.rotatedXPhase, size)
		p.rotatedYPhase = resizeFloat64(p.rotatedYPhase, size)
	}
	sin, cos := math.Sincos(p.rotation * math.Pi / 180)
	// Exact quarter turns avoid tiny errors at hard mask boundaries.
	switch p.rotation {
	case 90:
		sin, cos = 1, 0
	case 180:
		sin, cos = 0, -1
	case 270:
		sin, cos = -1, 0
	}

	for c := range p.xPhase {
		p.xPhase[c] = float64(c) * 0.18
	}
	for r := range p.yPhase {
		p.yPhase[r] = float64(r) * 0.22
	}
	cx := float64(p.columns) * 0.09
	cy := float64(p.rows) * 0.11
	for r := 0; r < p.rows; r++ {
		for c := 0; c < p.columns; c++ {
			position := r*p.columns + c
			sourceColumn, sourceRow := float64(c), float64(r)
			x, y := p.xPhase[c], p.yPhase[r]
			if p.rotation != 0 {
				// Inverse-map each output cell center into the unrotated field.
				dx := (float64(c) + .5 - float64(p.columns)/2) * ColumnWidth
				dy := (float64(r) + .5 - float64(p.rows)/2) * FontSize
				sourceColumn = (cos*dx+sin*dy)/ColumnWidth + float64(p.columns)/2 - .5
				sourceRow = (-sin*dx+cos*dy)/FontSize + float64(p.rows)/2 - .5
				x, y = sourceColumn*.18, sourceRow*.22
				p.rotatedXPhase[position], p.rotatedYPhase[position] = x, y
			}
			p.xyPhase[position] = (x + y) * 0.6
			dx, dy := sourceColumn-cx, sourceRow-cy
			p.distancePhase[position] = math.Sqrt(dx*dx+dy*dy) * 0.28
		}
	}
}

func resizeFloat64(values []float64, size int) []float64 {
	if cap(values) < size {
		return make([]float64, size)
	}
	return values[:size]
}

// Text renders the current frame as a plain character grid.
func (p *Component) Text() string { return p.Render().Text() }

// ANSI renders the current frame with true-color ANSI escape sequences.
func (p *Component) ANSI(foreground, background RGB) string {
	return p.Render().ANSI(foreground, background)
}

// ANSIAB renders the current frame with the coupled A/B color fade.
func (p *Component) ANSIAB(a, b, background RGB) string {
	return p.Render().ANSIAB(a, b, background)
}

// fixed3 matches toFixed(3) for the positive, finite alpha values produced here.
func fixed3(v float64) string {
	return strconv.FormatFloat(math.Floor(v*1000+0.5)/1000, 'f', 3, 64)
}
