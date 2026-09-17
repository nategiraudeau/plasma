package main

import (
	"math"
	"sort"
)

// A fixed working resolution keeps edits independent of terminal resizes.
// Only simplified vector contours are written to disk.
const rasterSize = 256

type canvas struct {
	pixels []bool
	edges  [4][rasterSize]bool
}

func newCanvas(contours [][]point) canvas {
	c := canvas{pixels: make([]bool, rasterSize*rasterSize)}
	// Scan each row once per edge, then fill even-odd spans.
	for y := 0; y < rasterSize; y++ {
		py := (float64(y) + .5) / rasterSize
		crossings := make([]float64, 0)
		for _, loop := range contours {
			if len(loop) < 3 {
				continue
			}
			a := loop[len(loop)-1]
			for _, b := range loop {
				if (a.Y > py) != (b.Y > py) {
					crossings = append(crossings, a.X+(py-a.Y)*(b.X-a.X)/(b.Y-a.Y))
				}
				a = b
			}
		}
		sort.Float64s(crossings)
		for i := 0; i+1 < len(crossings); i += 2 {
			start := max(0, int(math.Ceil(crossings[i]*rasterSize-.5)))
			end := min(rasterSize, int(math.Ceil(crossings[i+1]*rasterSize-.5)))
			for x := start; x < end; x++ {
				c.pixels[y*rasterSize+x] = true
			}
		}
	}
	return c
}
func (c canvas) at(x, y int) bool {
	return x >= 0 && x < rasterSize && y >= 0 && y < rasterSize && c.pixels[y*rasterSize+x]
}
func (c canvas) sample(x, y float64) bool {
	return c.at(min(rasterSize-1, int(x*rasterSize)), min(rasterSize-1, int(y*rasterSize)))
}

// Paint the entire segment, not just reported mouse positions, so fast drags
// leave continuous strokes. Geometry accounts for roughly 2:1 terminal cells.
func (c *canvas) stroke(a, b point, brush, width, height int, erase bool) {
	if width <= 0 || height <= 0 {
		return
	}
	sx, sy := float64(width), float64(height)*2
	radius := float64(brush) / 2
	lowX := max(0, int(math.Floor((math.Min(a.X, b.X)-radius/sx)*rasterSize)))
	highX := min(rasterSize-1, int(math.Ceil((math.Max(a.X, b.X)+radius/sx)*rasterSize)))
	lowY := max(0, int(math.Floor((math.Min(a.Y, b.Y)-radius/sy)*rasterSize)))
	highY := min(rasterSize-1, int(math.Ceil((math.Max(a.Y, b.Y)+radius/sy)*rasterSize)))
	aa, bb := point{X: a.X * sx, Y: a.Y * sy}, point{X: b.X * sx, Y: b.Y * sy}
	for y := lowY; y <= highY; y++ {
		for x := lowX; x <= highX; x++ {
			q := point{X: (float64(x) + .5) / rasterSize * sx, Y: (float64(y) + .5) / rasterSize * sy}
			if distance(q, aa, bb) <= radius {
				c.pixels[y*rasterSize+x] = !erase
			}
		}
	}
}

type vertex struct{ x, y int }
type edge struct {
	a, b vertex
	used bool
}

// Trace the boundary of the filled union. Interior stroke edges disappear;
// erased holes and disconnected islands become separate closed contours.
func (c canvas) contours() [][]point {
	edges := make([]edge, 0)
	outgoing := make(map[vertex][]int)
	add := func(a, b vertex) {
		outgoing[a] = append(outgoing[a], len(edges))
		edges = append(edges, edge{a: a, b: b})
	}
	for y := 0; y < rasterSize; y++ {
		for x := 0; x < rasterSize; x++ {
			if !c.at(x, y) {
				continue
			}
			if !c.at(x, y-1) {
				add(vertex{x, y}, vertex{x + 1, y})
			}
			if !c.at(x+1, y) {
				add(vertex{x + 1, y}, vertex{x + 1, y + 1})
			}
			if !c.at(x, y+1) {
				add(vertex{x + 1, y + 1}, vertex{x, y + 1})
			}
			if !c.at(x-1, y) {
				add(vertex{x, y + 1}, vertex{x, y})
			}
		}
	}
	loops := make([][]point, 0)
	for i := range edges {
		if edges[i].used {
			continue
		}
		start := edges[i].a
		loop := make([]point, 0)
		current := i
		for {
			e := &edges[current]
			e.used = true
			loop = append(loop, point{X: float64(e.a.x) / rasterSize, Y: float64(e.a.y) / rasterSize})
			if e.b == start {
				break
			}
			next, best := -1, -1
			dx, dy := e.b.x-e.a.x, e.b.y-e.a.y
			for _, candidate := range outgoing[e.b] {
				n := edges[candidate]
				if n.used {
					continue
				}
				nx, ny := n.b.x-n.a.x, n.b.y-n.a.y
				// Prefer the right turn at diagonal contacts; keep the two islands apart.
				score := 1
				if dx*ny-dy*nx > 0 {
					score = 3
				} else if dx*nx+dy*ny > 0 {
					score = 2
				}
				if score > best {
					next, best = candidate, score
				}
			}
			if next < 0 {
				break
			}
			current = next
		}
		if len(loop) >= 3 {
			loops = append(loops, simplifyLoop(loop))
		}
	}
	return loops
}

func distance(p, a, b point) float64 {
	dx, dy := b.X-a.X, b.Y-a.Y
	u := 0.0
	if n := dx*dx + dy*dy; n > 0 {
		u = math.Max(0, math.Min(1, ((p.X-a.X)*dx+(p.Y-a.Y)*dy)/n))
	}
	return math.Hypot(p.X-a.X-u*dx, p.Y-a.Y-u*dy)
}

// RDP at less than half a raster pixel removes staircase noise without moving
// a boundary across a pixel center. Split the ring at its farthest vertex.
func simplifyLoop(loop []point) []point {
	// Anchor viewport-edge vertices so simplified geometry meets the exact
	// intervals selected by the edge brush, including one-pixel corner turns.
	var anchors []int
	for i, p := range loop {
		if p.X == 0 || p.X == 1 || p.Y == 0 || p.Y == 1 {
			anchors = append(anchors, i)
		}
	}
	if len(anchors) == 0 {
		split, farthest := 1, 0.0
		for i := 1; i < len(loop); i++ {
			if d := math.Hypot(loop[i].X-loop[0].X, loop[i].Y-loop[0].Y); d > farthest {
				split, farthest = i, d
			}
		}
		anchors = []int{0, split}
	}
	result := make([]point, 0)
	for i, start := range anchors {
		end := anchors[(i+1)%len(anchors)]
		count := (end - start + len(loop)) % len(loop)
		if count == 0 {
			count = len(loop)
		}
		run := make([]point, count+1)
		for j := range run {
			run[j] = loop[(start+j)%len(loop)]
		}
		simple := simplify(run, .4/rasterSize)
		result = append(result, simple[:len(simple)-1]...)
	}
	if len(result) < 3 {
		return loop
	}
	// The arbitrary trace start may sit in the middle of a straight edge.
	// Remove that redundant vertex across the ring's closing seam as well.
	cleaned := make([]point, 0, len(result))
	for i, p := range result {
		if distance(p, result[(i+len(result)-1)%len(result)], result[(i+1)%len(result)]) > 1e-12 {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) < 3 {
		return result
	}
	return cleaned
}
func simplify(points []point, tolerance float64) []point {
	if len(points) < 3 {
		return points
	}
	maximum, index := 0.0, 0
	for i := 1; i < len(points)-1; i++ {
		if d := distance(points[i], points[0], points[len(points)-1]); d > maximum {
			maximum, index = d, i
		}
	}
	if maximum <= tolerance {
		return []point{points[0], points[len(points)-1]}
	}
	a, b := simplify(points[:index+1], tolerance), simplify(points[index:], tolerance)
	return append(a[:len(a)-1], b...)
}
