package plasma

import (
	"math"
	"reflect"
	"testing"
)

func TestConfigurableMaskMatchesOriginal(t *testing.T) {
	m := DefaultMask()
	for _, time := range []float64{0, 1.234, 20, 100} {
		for row := 0; row < 50; row++ {
			y := (float64(row) + .5) / 50
			l, r := maskEdges(y, time)
			ml, mr := m.edges(y, time)
			if l != ml || r != mr {
				t.Fatal("default edge motion changed")
			}
			for col := 0; col < 160; col++ {
				x := (float64(col) + .5) / 160
				if m.value(x, y, time, ml, mr) != horizontalMask(x, l, r) {
					t.Fatal("default mask changed")
				}
			}
		}
	}
}

func TestMaskPathAndResize(t *testing.T) {
	path := new(Path).MoveTo(.1, .1).LineTo(.9, .1).LineTo(.9, .9).LineTo(.1, .9)
	// Cut a hole out of the center using a second contour.
	path.MoveTo(.4, .4).LineTo(.6, .4).LineTo(.6, .6).LineTo(.4, .6)
	p := New(100, 50)
	if err := p.SetMask(Mask{Path: path}); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{100, 50}, {60, 20}} {
		p.Resize(size[0], size[1])
		f := p.Render()
		if len(f.Cells) == 0 {
			t.Fatal("empty custom region")
		}
		for _, c := range f.Cells {
			x, y := (float64(c.Column)+.5)/float64(size[0]), (float64(c.Row)+.5)/float64(size[1])
			if x <= .1 || x >= .9 || y <= .1 || y >= .9 || (x > .4 && x < .6 && y > .4 && y < .6) {
				t.Fatalf("cell outside path: %+v", c)
			}
		}
		fast := p.RenderFastInto(nil)
		for i, c := range f.Cells {
			if fast.Cells[i].Opacity != c.Opacity || fast.Cells[i].Glyph != c.Glyph {
				t.Fatal("fast path differs")
			}
		}
	}
	before := p.Render()
	path.LineTo(2, 2)
	if !reflect.DeepEqual(before, p.Render()) {
		t.Fatal("path not snapshotted")
	}
	p.SetTime(12)
	if err := p.SetMask(DefaultMask()); err != nil {
		t.Fatal(err)
	}
	if p.Time() != 12 {
		t.Fatal("configuration resets time")
	}
}

func TestMaskCurvesFeatherAndDistortion(t *testing.T) {
	path := new(Path).MoveTo(.1, .5).CubicTo(.1, 0, .9, 0, .9, .5).QuadTo(.5, 1, .1, .5)
	m := Mask{Path: path, Feather: .1}
	if m.value(.5, .5, 0, 0, 0) != 1 || m.value(.01, .5, 0, 0, 0) != 0 {
		t.Fatal("curve fill incorrect")
	}
	if m.value(.5, .5, 0, 0, 0) != m.value(.5, .5, 100, 0, 0) {
		t.Fatal("disabled distortion animates")
	}
	// A straight edge makes the inward feather distance exact.
	m.Path = new(Path).MoveTo(.2, 0).LineTo(1, 0).LineTo(1, 1).LineTo(.2, 1)
	v := m.value(.225, .5, 0, 0, 0)
	if v <= 0 || v >= .25 {
		t.Fatalf("expected soft S-curve: %v", v)
	}
	m.Feather = 0
	if m.value(.225, .5, 0, 0, 0) != 1 {
		t.Fatal("zero feather must be hard")
	}
	m.Feather = .1
	m.Distortion = 1
	if m.value(.225, .5, 0, 0, 0) == m.value(.225, .5, 10, 0, 0) {
		t.Fatal("distortion does not animate")
	}
	m = DefaultMask()
	m.Distortion = 0
	l, r := m.edges(.5, 100)
	if l != .42 || r != 1.07 {
		t.Fatal("band distortion not disabled")
	}
}

func TestMaskInvalidSettingsAndEmptyPath(t *testing.T) {
	p := New(80, 24)
	before := p.Render()
	for _, m := range []Mask{{Feather: -1}, {Distortion: -1}, {Feather: math.NaN()}, {Distortion: math.Inf(1)}, {Path: new(Path).MoveTo(math.NaN(), 0)}} {
		if p.SetMask(m) == nil {
			t.Fatal("invalid mask accepted")
		}
		if !reflect.DeepEqual(before, p.Render()) {
			t.Fatal("invalid mask changed component")
		}
	}
	if err := p.SetMask(Mask{Path: new(Path)}); err != nil {
		t.Fatal(err)
	}
	if len(p.Render().Cells) != 0 {
		t.Fatal("empty path should hide all cells")
	}
}

func TestCachedPathDistancesMatchDirectSampling(t *testing.T) {
	p := New(80, 24)
	path := new(Path).MoveTo(.1, .5).CubicTo(.1, 0, .9, 0, .9, .5).QuadTo(.5, 1, .1, .5)
	m := Mask{Path: path, Feather: .1, Distortion: 1.5}
	if err := p.SetMask(m); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{80, 24}, {100, 35}} {
		p.Resize(size[0], size[1])
		for _, time := range []float64{0, 10, 100} {
			for r := 0; r < p.rows; r++ {
				for c := 0; c < p.columns; c++ {
					x, y := (float64(c)+.5)/float64(p.columns), (float64(r)+.5)/float64(p.rows)
					got := p.mask.pathValue(p.maskDistance[r*p.columns+c], x, y, time)
					if want := m.value(x, y, time, 0, 0); got != want {
						t.Fatalf("cached mask=%v direct=%v", got, want)
					}
				}
			}
		}
	}
	if err := p.SetMask(DefaultMask()); err != nil {
		t.Fatal(err)
	}
	if len(p.maskDistance) != 0 {
		t.Fatal("restoring default retained stale geometry")
	}
}
