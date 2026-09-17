package main

import (
	tea "charm.land/bubbletea/v2"
	"math/rand"
	"plasma"
	"reflect"
	"strings"
	"testing"
)

func TestEdgeBrushOnAllBordersAndInInterior(t *testing.T) {
	for side := 0; side < 4; side++ {
		c := newCanvas(nil)
		endpoints := [4]point{{X: .5, Y: .5 / 27}, {X: 99.5 / 100, Y: .5}, {X: .5, Y: 26.5 / 27}, {X: .5 / 100, Y: .5}}
		q := endpoints[side]
		c.edgeStroke(q, q, 1, 100, 27, false)
		if !c.marked(side, .45, .55) {
			t.Fatalf("size-1 brush cannot mark side %d", side)
		}
		c.edgeStroke(q, q, 1, 100, 27, true)
		if c.edges != [4][rasterSize]bool{} {
			t.Fatal("eraser did not clear edge definitions")
		}
	}
	c := newCanvas(nil)
	c.edgeStroke(point{X: .5, Y: .5}, point{X: .6, Y: .5}, 5, 100, 27, false)
	if c.edges != [4][rasterSize]bool{} {
		t.Fatal("edge brush marked an edge from the interior")
	}
}
func TestEdgeBrushSaveReloadResizeAndErase(t *testing.T) {
	m := testModel(t)
	// Paint a thick region that reaches the right edge.
	m.doc.Brush = 15
	m.Update(tea.MouseClickMsg{X: 99, Y: 7, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 99, Y: 19, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 99, Y: 19, Button: tea.MouseLeft})
	pixels := append([]bool(nil), m.canvas.pixels...)
	press(m, 'b')
	if !m.edgeBrush || m.erase {
		t.Fatal("B did not select edge brush")
	}
	m.Update(tea.MouseClickMsg{X: 99, Y: 7, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 99, Y: 19, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 99, Y: 19, Button: tea.MouseLeft})
	if len(m.doc.Edges.Right) == 0 {
		t.Fatal("release did not persist edge spans")
	}
	if !reflect.DeepEqual(pixels, m.canvas.pixels) {
		t.Fatal("edge brush changed underlying paint")
	}
	if !strings.Contains(m.View().Content, "255;0;0m") {
		t.Fatal("edge marks aren't bright red")
	}
	original := m.doc.Edges
	ctrl(m, 's')
	d, _, err := loadDocument(m.filename)
	if err != nil || !reflect.DeepEqual(d.Edges, original) {
		t.Fatalf("edges didn't save: %v", err)
	}
	reopened := newModel(m.filename, d, false)
	if reopened.canvas.edges != m.canvas.edges {
		t.Fatal("edge marks didn't reload")
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 45})
	if !reflect.DeepEqual(m.doc.Edges, original) {
		t.Fatal("resize changed normalized edge definitions")
	}
	press(m, tea.KeyTab)
	press(m, tea.KeyTab)
	press(m, tea.KeyTab)
	if !reflect.DeepEqual(m.doc.Edges, original) {
		t.Fatal("screen switching lost edge definitions")
	}
	// One eraser removes both red edge definitions and the white shape under it.
	press(m, 'e')
	m.Update(tea.MouseClickMsg{X: 139, Y: 21, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 139, Y: 21, Button: tea.MouseLeft})
	if m.canvas.marked(rightEdge, .5, .51) || m.canvas.sample(139.5/140, 21.5/42) {
		t.Fatal("eraser failed to remove both red and white")
	}
	press(m, 'c')
	if m.canvas.edges != [4][rasterSize]bool{} || !reflect.DeepEqual(m.doc.Edges, plasma.EdgeExtensions{}) || len(m.doc.Contours) != 0 {
		t.Fatal("clear must clear shape and edge definitions")
	}
}
func TestEdgeBrushFooterFitsWithExplicitSizeKeys(t *testing.T) {
	m := testModel(t)
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
	for _, edgeMode := range []bool{false, true} {
		for _, erase := range []bool{false, true} {
			m.edgeBrush, m.erase = edgeMode, erase
			footer := strings.Join(m.footer(), "\n")
			for _, label := range []string{"Press -:", "Press +:", "B ", "E ", "C Clear", "^S Save", "^X Exit"} {
				if !strings.Contains(footer, label) {
					t.Fatalf("shortcut %q is clipped", label)
				}
			}
		}
	}
}
func TestEdgeAwareSimplificationPreservesPaint(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := 0; trial < 12; trial++ {
		c := newCanvas(nil)
		for stroke := 0; stroke < 8; stroke++ {
			c.stroke(point{X: rng.Float64(), Y: rng.Float64()}, point{X: 1, Y: rng.Float64()}, 1+rng.Intn(15), 100, 27, stroke%3 == 2)
		}
		if got := newCanvas(c.contours()); !reflect.DeepEqual(c.pixels, got.pixels) {
			t.Fatal("edge-aware vector simplification changed painted pixels")
		}
	}
}
