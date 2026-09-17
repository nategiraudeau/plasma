package plasma

import (
	"math"
	"reflect"
	"testing"
)

func rectanglePath(left, top, right, bottom float64) *Path {
	return new(Path).MoveTo(left, top).LineTo(right, top).LineTo(right, bottom).LineTo(left, bottom)
}
func configuredMask(t *testing.T, path *Path, e EdgeExtensions) Mask {
	t.Helper()
	p := New(0, 0)
	if err := p.SetMask(Mask{Path: path, Extensions: e, Feather: .1}); err != nil {
		t.Fatal(err)
	}
	return p.mask
}
func TestInfiniteEdgeExtensionRemovesOnlySelectedCutoff(t *testing.T) {
	path := rectanglePath(.3, .2, 1, .8)
	m := configuredMask(t, path, EdgeExtensions{Right: []EdgeSpan{{.2, .8}}})
	for _, x := range []float64{.99, 1, 1.1, 100, 1e12} {
		if d := m.distance(point{x, .5}); math.Abs(d-.3) > 1e-10 {
			t.Fatalf("x=%g distance=%g want .3", x, d)
		}
		if m.value(x, .5, 0, 0, 0) != 1 {
			t.Fatal("continued edge still fades")
		}
	}
	if m.distance(point{1e12, .9}) >= 0 {
		t.Fatal("extension escaped the selected interval")
	}
	if math.Abs(m.distance(point{.31, .5})-.01) > 1e-12 {
		t.Fatal("unmarked left edge changed")
	}
	partial := configuredMask(t, path, EdgeExtensions{Right: []EdgeSpan{{.4, .6}}})
	if math.Abs(partial.distance(point{1, .5})-.1) > 1e-12 {
		t.Fatal("partial edge has a seam")
	}
	if partial.distance(point{1.1, .3}) >= 0 {
		t.Fatal("unmarked edge extended")
	}
	if partial.distance(point{.99, .3}) > .011 {
		t.Fatal("unmarked edge lost its feather")
	}
}
func TestAllDirectionsCornersAndHoles(t *testing.T) {
	full := []EdgeSpan{{0, 1}}
	for side := 0; side < 4; side++ {
		var e EdgeExtensions
		switch side {
		case 0:
			e.Top = full
		case 1:
			e.Right = full
		case 2:
			e.Bottom = full
		case 3:
			e.Left = full
		}
		m := configuredMask(t, rectanglePath(0, 0, 1, 1), e)
		origin, dir := edgePoint(side, .5), edgeDirection(side)
		q := point{origin.x + dir.x*1e6, origin.y + dir.y*1e6}
		if math.Abs(m.distance(q)-.5) > 1e-12 {
			t.Fatalf("side %d did not extend", side)
		}
		// A single extended edge must not fill either outside corner quadrant.
		if m.distance(point{-1, -1}) >= 0 || m.distance(point{2, 2}) >= 0 {
			t.Fatal("single edge filled a corner quadrant")
		}
	}
	m := configuredMask(t, rectanglePath(0, 0, 1, 1), EdgeExtensions{Top: full, Right: full})
	if m.distance(point{1e6, -1e6}) <= 0 {
		t.Fatal("adjoining edges did not continue through corner")
	}
	all := EdgeExtensions{Top: full, Right: full, Bottom: full, Left: full}
	m = configuredMask(t, rectanglePath(0, 0, 1, 1), all)
	for _, q := range []point{{.5, .5}, {0, 0}, {1e12, 1e12}, {-1e12, -1e12}} {
		if !math.IsInf(m.distance(q), 1) {
			t.Fatalf("all extended edges should cover plane: %+v", q)
		}
	}
	path := rectanglePath(0, 0, 1, 1).MoveTo(.4, .4).LineTo(.6, .4).LineTo(.6, .6).LineTo(.4, .6)
	m = configuredMask(t, path, all)
	if m.distance(point{.5, .5}) >= 0 || m.distance(point{1e6, 1e6}) <= 0 {
		t.Fatal("extensions filled an erased hole")
	}
}
func TestEdgeMarksNeedPaintAndSnapshotTheirSpans(t *testing.T) {
	selected := []EdgeSpan{{0, 1}}
	path := rectanglePath(.2, .2, .8, .8)
	m := configuredMask(t, path, EdgeExtensions{Right: selected})
	for _, q := range []point{{.5, .5}, {.99, .5}, {1e6, .5}} {
		if m.distance(q) != path.distance(q) {
			t.Fatal("mark without touching paint changed mask")
		}
	}
	m = configuredMask(t, rectanglePath(.3, .2, 1, .8), EdgeExtensions{Right: selected})
	selected[0] = EdgeSpan{.9, 1}
	if m.distance(point{100, .5}) <= 0 {
		t.Fatal("caller mutated snapshotted edge spans")
	}
}
func TestInvalidExtensionsLeaveMaskUnchanged(t *testing.T) {
	p := New(80, 24)
	before := p.Render()
	for _, s := range []EdgeSpan{{-.1, .5}, {0, 2}, {.5, .5}, {.7, .2}, {math.NaN(), 1}, {0, math.Inf(1)}} {
		if err := p.SetMask(Mask{Path: rectanglePath(0, 0, 1, 1), Extensions: EdgeExtensions{Right: []EdgeSpan{s}}}); err == nil {
			t.Fatal("invalid span accepted")
		}
		if !reflect.DeepEqual(before, p.Render()) {
			t.Fatal("invalid span changed rendering")
		}
	}
	if p.SetMask(Mask{Extensions: EdgeExtensions{Right: []EdgeSpan{{0, 1}}}}) == nil {
		t.Fatal("extensions require a path")
	}
	if p.SetMask(Mask{Path: rectanglePath(0, 0, 2, 1), Extensions: EdgeExtensions{Right: []EdgeSpan{{0, 1}}}}) == nil {
		t.Fatal("extended path must fit viewport")
	}
}

func TestEvenOddBorderDoesNotExtendCancelledPaint(t *testing.T) {
	path := rectanglePath(0, 0, 1, 1).MoveTo(0, 0).LineTo(1, 0).LineTo(1, 1).LineTo(0, 1)
	m := configuredMask(t, path, EdgeExtensions{Right: []EdgeSpan{{0, 1}}})
	if m.distance(point{100, .5}) >= 0 {
		t.Fatal("cancelled contours extended outside viewport")
	}
}
