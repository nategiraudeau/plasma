package plasma

import (
	"fmt"
	"math"
)

// Mask configures a region relative to the component's current dimensions.
// Coordinates use 0..1 for each axis and may extend beyond the viewport.
// A nil Path selects the original full-height band from x=0.42 to x=1.07.
type Mask struct {
	Path *Path
	// Extensions opens selected viewport-edge intervals outward to infinity.
	Extensions EdgeExtensions
	boundary   *extendedBoundary
	// Feather is the inward gradient width in normalized coordinates (default .24).
	// Zero produces a hard edge.
	Feather float64
	// Distortion scales the animated edge displacement (default 1, zero disables it).
	Distortion float64
}

// DefaultMask returns the original mask and animation settings.
func DefaultMask() Mask { return Mask{Feather: .24, Distortion: 1} }

type point struct{ x, y float64 }

// Path is a vector path built using MoveTo, LineTo, QuadTo, and CubicTo.
// Subpaths are implicitly closed and filled with the even-odd rule, allowing
// holes and disjoint regions. Curves are flattened to within .0001 normalized
// units (with a maximum subdivision depth of 16).
// The zero value is an empty path; call MoveTo before drawing a subpath.
type Path struct{ contours [][]point }

// MoveTo begins a new subpath.
func (p *Path) MoveTo(x, y float64) *Path {
	p.contours = append(p.contours, []point{{x, y}})
	return p
}

// LineTo adds a straight segment. Without a prior MoveTo it begins a subpath.
func (p *Path) LineTo(x, y float64) *Path {
	if len(p.contours) == 0 {
		return p.MoveTo(x, y)
	}
	i := len(p.contours) - 1
	p.contours[i] = append(p.contours[i], point{x, y})
	return p
}

// QuadTo adds a quadratic Bézier segment with one control point.
func (p *Path) QuadTo(cx, cy, x, y float64) *Path {
	if len(p.contours) == 0 {
		p.MoveTo(0, 0)
	}
	a := p.last()
	return p.CubicTo(a.x+2*(cx-a.x)/3, a.y+2*(cy-a.y)/3,
		x+2*(cx-x)/3, y+2*(cy-y)/3, x, y)
}

// CubicTo adds a cubic Bézier segment with two control points.
// Without a prior MoveTo, curves start at (0,0).
func (p *Path) CubicTo(c1x, c1y, c2x, c2y, x, y float64) *Path {
	if len(p.contours) == 0 {
		p.MoveTo(0, 0)
	}
	p.flatten(p.last(), point{c1x, c1y}, point{c2x, c2y}, point{x, y}, 0)
	return p
}

func (p *Path) last() point {
	c := p.contours[len(p.contours)-1]
	return c[len(c)-1]
}

func midpoint(a, b point) point { return point{(a.x + b.x) / 2, (a.y + b.y) / 2} }
func (p *Path) flatten(a, b, c, d point, depth int) {
	if !finite(a.x) || !finite(a.y) || !finite(b.x) || !finite(b.y) || !finite(c.x) || !finite(c.y) || !finite(d.x) || !finite(d.y) {
		p.LineTo(math.NaN(), math.NaN())
		return
	}
	if depth == 16 || math.Max(segmentDistance(b, a, d), segmentDistance(c, a, d)) <= .0001 {
		p.LineTo(d.x, d.y)
		return
	}
	ab, bc, cd := midpoint(a, b), midpoint(b, c), midpoint(c, d)
	abc, bcd := midpoint(ab, bc), midpoint(bc, cd)
	m := midpoint(abc, bcd)
	p.flatten(a, ab, abc, m, depth+1)
	p.flatten(m, bcd, cd, d, depth+1)
}

// SetMask changes the mask without resetting animation time or dimensions.
// It snapshots the path, so later builder edits cannot change this component.
// Invalid settings return an error and leave the previous mask unchanged.
func (p *Component) SetMask(mask Mask) error {
	if !finite(mask.Feather) || mask.Feather < 0 || !finite(mask.Distortion) || mask.Distortion < 0 {
		return fmt.Errorf("mask feather and distortion must be finite and nonnegative")
	}
	if mask.Path != nil {
		copyPath := &Path{}
		for _, contour := range mask.Path.contours {
			for _, v := range contour {
				if !finite(v.x) || !finite(v.y) {
					return fmt.Errorf("mask path coordinates must be finite")
				}
			}
			copyPath.contours = append(copyPath.contours, append([]point(nil), contour...))
		}
		mask.Path = copyPath
	}
	extensions, err := mask.Extensions.snapshot()
	if err != nil {
		return err
	}
	mask.Extensions = extensions
	mask.boundary = nil
	if extensions.any() {
		if mask.Path == nil {
			return fmt.Errorf("edge extensions require a custom path")
		}
		for _, contour := range mask.Path.contours {
			for _, q := range contour {
				if q.x < 0 || q.x > 1 || q.y < 0 || q.y > 1 {
					return fmt.Errorf("an extended path must lie within the normalized viewport")
				}
			}
		}
		mask.boundary = compileExtensions(mask.Path, extensions)
	}
	p.mask = mask
	p.rebuildMaskGeometry()
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func featherMask(distance, width float64) float64 {
	if distance <= 0 {
		return 0
	}
	if width == 0 {
		return 1
	}
	return smootherstep(distance / width)
}

func (m Mask) edges(y, t float64) (float64, float64) {
	// Preserve the original operation order for bit-for-bit default output.
	return .42 + m.Distortion*.008*math.Sin(y*5+t*.23) + m.Distortion*.004*math.Sin(y*9-t*.17),
		1.07 + m.Distortion*.008*math.Sin(y*4.3-t*.19+1.7) + m.Distortion*.004*math.Sin(y*7+t*.13)
}

func (m Mask) value(x, y, t, left, right float64) float64 {
	if m.Path == nil {
		return featherMask(x-left, m.Feather) * featherMask(right-x, m.Feather)
	}
	return m.pathValue(m.distance(point{x, y}), x, y, t)
}

func (m Mask) pathValue(distance, x, y, t float64) float64 {
	// Spatially varying normal displacement deforms arbitrary boundaries.
	if m.Distortion != 0 {
		distance += m.Distortion * (.008*math.Sin(y*5+x*4.3+t*.23) + .004*math.Sin(y*9-x*7-t*.17))
	}
	return featherMask(distance, m.Feather)
}

func (p *Path) distance(q point) float64 {
	inside := false
	distance := math.Inf(1)
	for _, contour := range p.contours {
		if len(contour) < 3 {
			continue
		}
		a := contour[len(contour)-1]
		for _, b := range contour {
			distance = math.Min(distance, segmentDistance(q, a, b))
			if (a.y > q.y) != (b.y > q.y) && q.x < (b.x-a.x)*(q.y-a.y)/(b.y-a.y)+a.x {
				inside = !inside
			}
			a = b
		}
	}
	if !inside {
		return -distance
	}
	return distance
}

func segmentDistance(q, a, b point) float64 {
	dx, dy := b.x-a.x, b.y-a.y
	length := dx*dx + dy*dy
	u := 0.0
	if length > 0 {
		u = math.Max(0, math.Min(1, ((q.x-a.x)*dx+(q.y-a.y)*dy)/length))
	}
	return math.Hypot(q.x-a.x-u*dx, q.y-a.y-u*dy)
}

// Cache static path distances so frame cost does not grow with path complexity.
func (p *Component) rebuildMaskGeometry() {
	if p.mask.Path == nil {
		p.maskDistance = p.maskDistance[:0]
		return
	}
	p.maskDistance = resizeFloat64(p.maskDistance, p.columns*p.rows)
	for r := 0; r < p.rows; r++ {
		y := (float64(r) + .5) / float64(p.rows)
		for c := 0; c < p.columns; c++ {
			x := (float64(c) + .5) / float64(p.columns)
			p.maskDistance[r*p.columns+c] = p.mask.distance(point{x, y})
		}
	}
}
