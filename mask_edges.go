package plasma

import (
	"fmt"
	"math"
	"sort"
)

// EdgeSpan selects a normalized interval along a viewport edge. Top and bottom
// run left to right; left and right run top to bottom.
type EdgeSpan struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// EdgeExtensions continues a custom mask infinitely outward where its filled
// path touches the selected edge intervals. Its path must lie within [0,1]².
// Adjacent extensions meeting at a corner also continue through that quadrant.
// Unmarked boundaries, including holes, keep their normal feather and distortion.
type EdgeExtensions struct {
	Top    []EdgeSpan `json:"top,omitempty"`
	Right  []EdgeSpan `json:"right,omitempty"`
	Bottom []EdgeSpan `json:"bottom,omitempty"`
	Left   []EdgeSpan `json:"left,omitempty"`
}

func (e EdgeExtensions) sides() [4][]EdgeSpan { return [4][]EdgeSpan{e.Top, e.Right, e.Bottom, e.Left} }
func (e EdgeExtensions) any() bool            { return len(e.Top)+len(e.Right)+len(e.Bottom)+len(e.Left) > 0 }
func mergeSpans(spans []EdgeSpan) []EdgeSpan {
	if len(spans) == 0 {
		return nil
	}
	spans = append([]EdgeSpan(nil), spans...)
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	result := spans[:0]
	for _, span := range spans {
		if len(result) > 0 && span.Start <= result[len(result)-1].End {
			result[len(result)-1].End = math.Max(result[len(result)-1].End, span.End)
		} else {
			result = append(result, span)
		}
	}
	return result
}

// Along a clipped viewport border the contour crossings follow the same
// even-odd fill rule as the path, so overlapping spans cancel in pairs.
func filledBorderSpans(spans []EdgeSpan) []EdgeSpan {
	endpoints := make([]float64, 0, 2*len(spans))
	for _, s := range spans {
		endpoints = append(endpoints, s.Start, s.End)
	}
	sort.Float64s(endpoints)
	result := make([]EdgeSpan, 0)
	for i := 0; i+1 < len(endpoints); i += 2 {
		if endpoints[i] < endpoints[i+1] {
			result = append(result, EdgeSpan{endpoints[i], endpoints[i+1]})
		}
	}
	return mergeSpans(result)
}

func (e EdgeExtensions) snapshot() (EdgeExtensions, error) {
	sides := e.sides()
	for i, spans := range sides {
		for _, s := range spans {
			if !finite(s.Start) || !finite(s.End) || s.Start < 0 || s.End > 1 || s.Start >= s.End {
				return EdgeExtensions{}, fmt.Errorf("edge spans must satisfy 0 <= start < end <= 1")
			}
		}
		sides[i] = mergeSpans(spans)
	}
	return EdgeExtensions{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}, nil
}

type boundarySegment struct{ a, b point }
type boundaryRay struct{ origin, direction point }
type extendedBoundary struct {
	segments []boundarySegment
	rays     []boundaryRay
	sides    [4][]EdgeSpan
}

func edgePoint(side int, t float64) point {
	switch side {
	case 0:
		return point{t, 0}
	case 1:
		return point{1, t}
	case 2:
		return point{t, 1}
	default:
		return point{0, t}
	}
}
func edgeDirection(side int) point {
	switch side {
	case 0:
		return point{0, -1}
	case 1:
		return point{1, 0}
	case 2:
		return point{0, 1}
	default:
		return point{-1, 0}
	}
}
func onEdge(a, b point) (int, float64, float64) {
	switch {
	case a.y == 0 && b.y == 0:
		return 0, math.Min(a.x, b.x), math.Max(a.x, b.x)
	case a.x == 1 && b.x == 1:
		return 1, math.Min(a.y, b.y), math.Max(a.y, b.y)
	case a.y == 1 && b.y == 1:
		return 2, math.Min(a.x, b.x), math.Max(a.x, b.x)
	case a.x == 0 && b.x == 0:
		return 3, math.Min(a.y, b.y), math.Max(a.y, b.y)
	}
	return -1, 0, 0
}
func containsSpan(spans []EdgeSpan, t float64) bool {
	for _, s := range spans {
		if t >= s.Start && t <= s.End {
			return true
		}
	}
	return false
}
func (b *extendedBoundary) joinedCorner(side int, t float64) bool {
	if (t != 0 && t != 1) || !containsSpan(b.sides[side], t) {
		return false
	}
	q := edgePoint(side, t)
	if side == 0 || side == 2 {
		adjacent := 3
		if q.x == 1 {
			adjacent = 1
		}
		return containsSpan(b.sides[adjacent], q.y)
	}
	adjacent := 0
	if q.y == 1 {
		adjacent = 2
	}
	return containsSpan(b.sides[adjacent], q.x)
}

// Replace selected boundary segments with actual infinite rays. No artificial
// far-away closing polygon, and no seam remains at the viewport boundary.
func compileExtensions(path *Path, extensions EdgeExtensions) *extendedBoundary {
	b := &extendedBoundary{}
	selected := extensions.sides()
	var border [4][]EdgeSpan
	for _, contour := range path.contours {
		if len(contour) < 3 {
			continue
		}
		a := contour[len(contour)-1]
		for _, next := range contour {
			side, start, end := onEdge(a, next)
			if side < 0 {
				b.segments = append(b.segments, boundarySegment{a, next})
			} else if start < end {
				border[side] = append(border[side], EdgeSpan{start, end})
			}
			a = next
		}
	}
	for side := range border {
		border[side] = filledBorderSpans(border[side])
		for _, paint := range border[side] {
			for _, mark := range selected[side] {
				start, end := math.Max(paint.Start, mark.Start), math.Min(paint.End, mark.End)
				if start < end {
					b.sides[side] = append(b.sides[side], EdgeSpan{start, end})
				}
			}
		}
		b.sides[side] = mergeSpans(b.sides[side])
		for _, paint := range border[side] {
			cursor := paint.Start
			for _, span := range b.sides[side] {
				if span.End <= cursor || span.Start >= paint.End {
					continue
				}
				if cursor < span.Start {
					b.segments = append(b.segments, boundarySegment{edgePoint(side, cursor), edgePoint(side, span.Start)})
				}
				cursor = math.Max(cursor, span.End)
			}
			if cursor < paint.End {
				b.segments = append(b.segments, boundarySegment{edgePoint(side, cursor), edgePoint(side, paint.End)})
			}
		}
	}
	for side, spans := range b.sides {
		for _, s := range spans {
			for _, end := range []float64{s.Start, s.End} {
				if !b.joinedCorner(side, end) {
					b.rays = append(b.rays, boundaryRay{edgePoint(side, end), edgeDirection(side)})
				}
			}
		}
	}
	return b
}
func (m Mask) distance(q point) float64 {
	if m.boundary == nil {
		return m.Path.distance(q)
	}
	b := m.boundary
	inside := m.Path.distance(q) > 0
	if q.y <= 0 && containsSpan(b.sides[0], q.x) || q.x >= 1 && containsSpan(b.sides[1], q.y) || q.y >= 1 && containsSpan(b.sides[2], q.x) || q.x <= 0 && containsSpan(b.sides[3], q.y) {
		inside = true
	}
	// When both adjoining edges extend, their common corner has no boundary.
	if q.x <= 0 && q.y <= 0 && b.joinedCorner(0, 0) || q.x >= 1 && q.y <= 0 && b.joinedCorner(0, 1) || q.x >= 1 && q.y >= 1 && b.joinedCorner(2, 1) || q.x <= 0 && q.y >= 1 && b.joinedCorner(2, 0) {
		inside = true
	}
	d := math.Inf(1)
	for _, s := range b.segments {
		d = math.Min(d, segmentDistance(q, s.a, s.b))
	}
	for _, r := range b.rays {
		along := math.Max(0, (q.x-r.origin.x)*r.direction.x+(q.y-r.origin.y)*r.direction.y)
		d = math.Min(d, math.Hypot(q.x-r.origin.x-along*r.direction.x, q.y-r.origin.y-along*r.direction.y))
	}
	if !inside {
		return -d
	}
	return d
}
