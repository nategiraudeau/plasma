package main

import (
	"math"
	"plasma"
)

const (
	topEdge = iota
	rightEdge
	bottomEdge
	leftEdge
)

func (c *canvas) loadEdges(e plasma.EdgeExtensions) {
	sides := [4][]plasma.EdgeSpan{e.Top, e.Right, e.Bottom, e.Left}
	for side, spans := range sides {
		for i := 0; i < rasterSize; i++ {
			t := (float64(i) + .5) / rasterSize
			for _, s := range spans {
				if t >= s.Start && t < s.End {
					c.edges[side][i] = true
					break
				}
			}
		}
	}
}
func (c canvas) extensions() plasma.EdgeExtensions {
	var sides [4][]plasma.EdgeSpan
	for side, pixels := range c.edges {
		for i := 0; i < rasterSize; {
			if !pixels[i] {
				i++
				continue
			}
			start := i
			for i < rasterSize && pixels[i] {
				i++
			}
			sides[side] = append(sides[side], plasma.EdgeSpan{Start: float64(start) / rasterSize, End: float64(i) / rasterSize})
		}
	}
	return plasma.EdgeExtensions{Top: sides[0], Right: sides[1], Bottom: sides[2], Left: sides[3]}
}

// Use the same brush footprint for edge painting and erasing. The border is
// targeted at the outermost terminal cell centers, so even a size-1 brush can
// select it. Edge painting does not change the underlying white shape.
func (c *canvas) edgeStroke(a, b point, brush, width, height int, erase bool) {
	if width <= 0 || height <= 0 {
		return
	}
	sx, sy := float64(width), float64(height)*2
	aa, bb := point{X: a.X * sx, Y: a.Y * sy}, point{X: b.X * sx, Y: b.Y * sy}
	radius := float64(brush) / 2
	for i := 0; i < rasterSize; i++ {
		t := (float64(i) + .5) / rasterSize
		points := [4]point{{X: t * sx, Y: 1}, {X: sx - .5, Y: t * sy}, {X: t * sx, Y: sy - 1}, {X: .5, Y: t * sy}}
		for side, q := range points {
			if distance(q, aa, bb) <= radius {
				c.edges[side][i] = !erase
			}
		}
	}
}
func (c canvas) marked(side int, start, end float64) bool {
	// Cover any marker in the displayed edge cell, not only its midpoint: narrow
	// marks must remain visible after resizing to a smaller terminal.
	low := max(0, int(math.Floor(start*rasterSize)))
	high := min(rasterSize, int(math.Ceil(end*rasterSize)))
	for i := low; i < high; i++ {
		if c.edges[side][i] {
			return true
		}
	}
	return false
}

// canvasTone returns black, white, or bright red for one half of a terminal cell.
func (m *model) canvasTone(x, y int, lower bool) int {
	half := 0.0
	if lower {
		half = 1
	}
	px := (float64(x) + .5) / float64(m.width)
	py := (float64(y) + (half+.5)/2) / float64(m.canvasHeight())
	x0, x1 := float64(x)/float64(m.width), float64(x+1)/float64(m.width)
	y0, y1 := (float64(y)+half/2)/float64(m.canvasHeight()), (float64(y)+(half+1)/2)/float64(m.canvasHeight())
	if y == 0 && !lower && m.canvas.marked(topEdge, x0, x1) || y == m.canvasHeight()-1 && lower && m.canvas.marked(bottomEdge, x0, x1) || x == 0 && m.canvas.marked(leftEdge, y0, y1) || x == m.width-1 && m.canvas.marked(rightEdge, y0, y1) {
		return 2
	}
	if m.canvas.sample(px, py) {
		return 1
	}
	return 0
}
