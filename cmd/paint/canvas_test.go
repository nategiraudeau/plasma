package main

import (
	"reflect"
	"testing"
)

func TestTraceFilledUnionHolesAndIslands(t *testing.T) {
	c := newCanvas(nil)
	for y := 20; y < 120; y++ {
		for x := 20; x < 160; x++ {
			c.pixels[y*rasterSize+x] = true
		}
	}
	for y := 40; y < 80; y++ {
		for x := 40; x < 80; x++ {
			c.pixels[y*rasterSize+x] = false
		}
	}
	c.pixels[180*rasterSize+180] = true
	// Two diagonal pixels must be distinct rings, not one bow-tie contour.
	c.pixels[181*rasterSize+181] = true
	loops := c.contours()
	if len(loops) != 4 {
		t.Fatalf("got %d contours, want outer + hole + 2 islands", len(loops))
	}
	for _, loop := range loops {
		if len(loop) != 4 {
			t.Fatalf("rectangle wasn't simplified: %d vertices", len(loop))
		}
	}
	restored := newCanvas(loops)
	if !reflect.DeepEqual(c.pixels, restored.pixels) {
		t.Fatal("trace/fill changed painted region or erased hole")
	}
	if !reflect.DeepEqual(loops, c.contours()) {
		t.Fatal("vector output is nondeterministic")
	}
}

func TestPaintFastDragEraseAndSimplify(t *testing.T) {
	c := newCanvas(nil)
	c.stroke(point{X: .1, Y: .5}, point{X: .9, Y: .5}, 9, 100, 30, false)
	for _, x := range []float64{.1, .3, .5, .7, .9} {
		if !c.sample(x, .5) {
			t.Fatal("fast drag left a gap")
		}
	}
	loops := c.contours()
	vertices := 0
	for _, loop := range loops {
		vertices += len(loop)
	}
	if len(loops) != 1 || vertices >= 200 {
		t.Fatalf("stroke did not simplify: %d contours, %d vertices", len(loops), vertices)
	}
	c.stroke(point{X: .5, Y: .5}, point{X: .5, Y: .5}, 3, 100, 30, true)
	if c.sample(.5, .5) {
		t.Fatal("erase failed")
	}
	if len(c.contours()) != 2 {
		t.Fatal("erased interior should produce a hole")
	}
	restored := newCanvas(c.contours())
	// Every raster pixel retains its inside/outside classification.
	if !reflect.DeepEqual(c.pixels, restored.pixels) {
		t.Fatal("simplification changed fill")
	}
}

func TestTraceEmptyAndFullCanvas(t *testing.T) {
	c := newCanvas(nil)
	if len(c.contours()) != 0 {
		t.Fatal("empty canvas has contours")
	}
	for i := range c.pixels {
		c.pixels[i] = true
	}
	loops := c.contours()
	if len(loops) != 1 || len(loops[0]) != 4 {
		t.Fatal("full canvas must simplify to a rectangle")
	}
	if !reflect.DeepEqual(c.pixels, newCanvas(loops).pixels) {
		t.Fatal("full canvas lost boundary")
	}
}
