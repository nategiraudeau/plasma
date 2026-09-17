package plasma

import (
	"math"
	"reflect"
	"testing"
)

func TestRotationZeroWrappingAndValidation(t *testing.T) {
	p := New(100, 30)
	p.SetTime(2.5)
	original := p.Render()
	for _, angle := range []float64{0, 360, -720} {
		if err := p.SetRotation(angle); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, p.Render()) {
			t.Fatalf("angle %v changed original output", angle)
		}
	}
	if err := p.SetRotation(-30); err != nil {
		t.Fatal(err)
	}
	if p.Rotation() != 330 || p.Time() != 2.5 {
		t.Fatal("angle wrapping or time preservation failed")
	}
	rotated := p.Render()
	if reflect.DeepEqual(original, rotated) {
		t.Fatal("rotation did not affect rendering")
	}
	for _, angle := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if p.SetRotation(angle) == nil {
			t.Fatal("invalid rotation accepted")
		}
		if !reflect.DeepEqual(rotated, p.Render()) {
			t.Fatal("invalid angle changed output")
		}
	}
	p.SetRotation(0)
	if !reflect.DeepEqual(original, p.Render()) {
		t.Fatal("reset to zero did not restore exact output")
	}
}

func TestHalfTurnRotatesWaves(t *testing.T) {
	// An unbounded hard mask isolates the wave field from screen clipping.
	full := Mask{Path: new(Path).MoveTo(-100, -100).LineTo(100, -100).LineTo(100, 100).LineTo(-100, 100)}
	for _, mask := range []Mask{full} {
		for _, size := range [][2]int{{80, 24}, {101, 35}} {
			p := New(size[0], size[1])
			p.SetRotation(180)
			// Setting a new path after rotating must also update its cached distances.
			if err := p.SetMask(mask); err != nil {
				t.Fatal(err)
			}
			original := New(size[0], size[1])
			original.SetMask(mask)
			for _, time := range []float64{0, 1.234, 25} {
				p.SetTime(time)
				original.SetTime(time)
				base, rotated := original.Render(), p.Render()
				if len(base.Cells) != len(rotated.Cells) {
					t.Fatal("half-turn changed visible cell count")
				}
				byPosition := make(map[[2]int]Cell)
				for _, c := range base.Cells {
					byPosition[[2]int{c.Column, c.Row}] = c
				}
				for _, c := range rotated.Cells {
					source, ok := byPosition[[2]int{size[0] - 1 - c.Column, size[1] - 1 - c.Row}]
					if !ok || source.Glyph != c.Glyph || math.Abs(source.Opacity-c.Opacity) > 1e-13 {
						t.Fatalf("half-turn didn't rotate full field: source=%+v output=%+v", source, c)
					}
				}
			}
		}
	}
}

func TestQuarterTurnAndArbitraryRotationPreservePhysicalDistance(t *testing.T) {
	p := New(100, 30)
	mask := Mask{Path: new(Path).MoveTo(.65, .4).LineTo(.75, .4).LineTo(.75, .6).LineTo(.65, .6)}
	p.SetMask(mask)
	for _, angle := range []float64{37, 90, 270} {
		p.SetRotation(angle)
		for r := 0; r < p.rows; r++ {
			for c := 0; c < p.columns; c++ {
				i := r*p.columns + c
				outX := (float64(c) + .5 - float64(p.columns)/2) * ColumnWidth
				outY := (float64(r) + .5 - float64(p.rows)/2) * FontSize
				srcX := (p.rotatedXPhase[i]/.18 + .5 - float64(p.columns)/2) * ColumnWidth
				srcY := (p.rotatedYPhase[i]/.22 + .5 - float64(p.rows)/2) * FontSize
				if math.Abs(math.Hypot(outX, outY)-math.Hypot(srcX, srcY)) > 1e-10 {
					t.Fatal("rotation stretched the field")
				}
				if angle == 90 && (math.Abs(srcX-outY) > 1e-10 || math.Abs(srcY+outX) > 1e-10) {
					t.Fatal("positive angle should rotate clockwise")
				}
				if p.maskDistance[i] != p.mask.Path.distance(point{(float64(c) + .5) / float64(p.columns), (float64(r) + .5) / float64(p.rows)}) {
					t.Fatal("path cache must stay in screen coordinates")
				}
			}
		}
	}

}

func TestRotatedResizeAndFastRendering(t *testing.T) {
	p := New(0, 0)
	p.SetRotation(32.5)
	p.Resize(90, 33)
	other := New(90, 33)
	other.SetRotation(32.5)
	if !reflect.DeepEqual(p.Render(), other.Render()) {
		t.Fatal("resize didn't rebuild rotated geometry")
	}
	full, fast := p.Render(), p.RenderFastInto(nil)
	if len(full.Cells) != len(fast.Cells) {
		t.Fatal("fast render differs")
	}
	for i, c := range full.Cells {
		if c.Opacity != fast.Cells[i].Opacity || c.Glyph != fast.Cells[i].Glyph {
			t.Fatal("fast render differs")
		}
	}
	cells := make([]Cell, 0, 90*33)
	if allocs := testing.AllocsPerRun(10, func() { p.RenderFastInto(cells) }); allocs != 0 {
		t.Fatalf("rotated fast path allocates: %v", allocs)
	}
}

func TestRotationKeepsMaskFeatherAndDistortionFixed(t *testing.T) {
	custom := DefaultMask()
	custom.Path = new(Path).MoveTo(.2, .2).LineTo(.85, .3).LineTo(.7, .9).LineTo(.3, .7)
	custom.Feather = .06
	full := Mask{Path: new(Path).MoveTo(-100, -100).LineTo(100, -100).LineTo(100, 100).LineTo(-100, 100)}
	for _, mask := range []Mask{DefaultMask(), custom} {
		p, reference := New(100, 35), New(100, 35)
		p.SetMask(mask)
		reference.SetMask(full)
		distances := append([]float64(nil), p.maskDistance...)
		for _, angle := range []float64{0, 37, 90, 180, 270} {
			p.SetRotation(angle)
			reference.SetRotation(angle)
			if !reflect.DeepEqual(distances, p.maskDistance) {
				t.Fatal("rotation changed mask geometry")
			}
			for _, time := range []float64{0, 1.234, 25} {
				p.SetTime(time)
				reference.SetTime(time)
				unmasked := make(map[[2]int]Cell)
				for _, cell := range reference.Render().Cells {
					unmasked[[2]int{cell.Column, cell.Row}] = cell
				}
				frame := p.Render()
				if len(frame.Cells) == 0 {
					t.Fatal("rotation hid the entire mask")
				}
				for _, cell := range frame.Cells {
					x, y := (float64(cell.Column)+.5)/100, (float64(cell.Row)+.5)/35
					left, right := mask.edges(y, time)
					weight := mask.value(x, y, time, left, right)
					source, ok := unmasked[[2]int{cell.Column, cell.Row}]
					if !ok || weight == 0 || math.Abs(cell.Opacity-source.Opacity*weight) > 1e-14 {
						t.Fatal("rotation changed mask feather or distortion")
					}
				}
			}
		}
	}
}
