package plasma

import (
	"bytes"
	"math"
	"testing"
)

func TestMaskBoundsAndFullHeight(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {160, 50}, {375, 1}} {
		p := New(size[0], size[1])
		for _, time := range []float64{0, 0.016, 1.234, 20, 100} {
			p.SetTime(time)
			f := p.Render()
			rows := make([]bool, f.Rows)
			for _, cell := range f.Cells {
				x := (float64(cell.Column) + 0.5) / float64(f.Columns)
				if x < 0.408 || x >= 1 {
					t.Fatalf("cell outside warped band: %+v", cell)
				}
				if cell.Opacity <= 0 || cell.Opacity > maxOpacity || cell.Alpha != fixed3(cell.Opacity) {
					t.Fatalf("invalid masked alpha: %+v", cell)
				}
				rows[cell.Row] = true
			}
			for row, visible := range rows {
				if !visible {
					t.Fatalf("%v t=%v: entire row %d masked out", size, time, row)
				}
			}
			fast := p.RenderFastInto(nil)
			if fast.Text() != f.Text() || len(fast.Cells) != len(f.Cells) {
				t.Fatal("fast render differs from regular render")
			}
			for i := range f.Cells {
				if fast.Cells[i].Opacity != f.Cells[i].Opacity {
					t.Fatal("fast render opacity differs")
				}
			}
		}
	}
}

func TestMaskSmoothFadeAndIndependentWarp(t *testing.T) {
	left, right := maskEdges(0.5, 0)
	if horizontalMask(left, left, right) != 0 || horizontalMask(right, left, right) != 0 || horizontalMask(0.70, left, right) != 1 {
		t.Fatal("mask must vanish at edges and reach full strength in center")
	}
	// A curved shoulder starts below a linear ramp and finishes above it.
	for _, edge := range []float64{left, right} {
		direction := 1.0
		if edge == right {
			direction = -1
		}
		quarter := horizontalMask(edge+direction*0.06, left, right)
		threeQuarters := horizontalMask(edge+direction*0.18, left, right)
		if quarter >= 0.25 || threeQuarters <= 0.75 {
			t.Fatal("feather must have an eased S-curve, not a linear falloff")
		}
	}
	previous := 0.0
	for i := 0; i <= 100; i++ {
		value := horizontalMask(left+0.24*float64(i)/100, left, right)
		if value < previous || value-previous > 0.02 {
			t.Fatal("left feather is not a gradual monotonic fade")
		}
		previous = value
	}
	// The viewport cuts through the right fade while it is still visible.
	for _, time := range []float64{0, 5, 20, 100} {
		for _, y := range []float64{0, 0.5, 1} {
			l, r := maskEdges(y, time)
			if value := horizontalMask(1, l, r); value < 0.09 || value > 0.23 {
				t.Fatalf("right viewport boundary should cut through the fade, got %v", value)
			}
		}
	}
	l2, r2 := maskEdges(0.5, 5)
	l3, r3 := maskEdges(0.9, 0)
	if left == l2 || right == r2 || left == l3 || right == r3 || l2-left == r2-right {
		t.Fatal("edges must warp independently over time and height")
	}
}

func TestMaskFadesDensityColorAndOpacity(t *testing.T) {
	p := New(200, 24)
	f := p.Render()
	faded := 0
	for _, cell := range f.Cells {
		position := cell.Row*p.columns + cell.Column
		v := (math.Sin(p.xPhase[cell.Column]) + math.Sin(p.yPhase[cell.Row]) +
			math.Sin(p.xyPhase[position]) + math.Sin(p.distancePhase[position])) / 4
		norm := (v + 1) / 2
		originalGlyph := densityRunes[int(math.Floor(norm*float64(len(densityRunes)-1)))]
		originalOpacity := math.Sin(norm*math.Pi) * maxOpacity
		if cell.Opacity > originalOpacity || densityMixRatio(cell.Glyph) > densityMixRatio(originalGlyph) {
			t.Fatal("mask increased density/color or opacity")
		}
		if cell.Opacity < originalOpacity && densityMixRatio(cell.Glyph) < densityMixRatio(originalGlyph) {
			faded++
		}
	}
	if faded == 0 {
		t.Fatal("mask did not affect both A/B mix and opacity")
	}
}

func TestStepAndPixelSizing(t *testing.T) {
	p := NewFromPixels(280, 140)
	if f := p.Render(); f.Columns != 32 || f.Rows != 10 {
		t.Fatalf("pixel sizing produced %dx%d, want 32x10", f.Columns, f.Rows)
	}
	p.Step()
	if p.Time() != FrameStep {
		t.Fatalf("time after step = %v, want %v", p.Time(), FrameStep)
	}
}

func TestANSIFakeOpacityUsesSuppliedColors(t *testing.T) {
	frame := Frame{
		Columns: 1,
		Rows:    1,
		Cells: []Cell{{
			Glyph:   '#',
			Opacity: maxOpacity / 2,
		}},
	}
	got := frame.ANSI(
		RGB{R: 100, G: 200, B: 50},
		RGB{R: 20, G: 40, B: 60},
	)
	want := "\x1b[48;2;20;40;60m\x1b[38;2;60;120;55m#\x1b[0m"
	if got != want {
		t.Fatalf("ANSI fake opacity:\n got %q\nwant %q", got, want)
	}
}

func TestANSIABMixFollowsDensityIndependentOfOpacity(t *testing.T) {
	// '#' sits at ramp index 8 of 11 visible glyphs, so its A/B mix is fixed
	// at 0.7 regardless of opacity; only the outer alpha (0.5 here) fades it
	// toward the background.
	frame := Frame{Columns: 1, Rows: 1, Cells: []Cell{{Glyph: '#', Opacity: maxOpacity / 2}}}
	got := frame.ANSIAB(
		RGB{R: 255, G: 0, B: 0},
		RGB{R: 0, G: 0, B: 255},
		RGB{},
	)
	want := "\x1b[48;2;0;0;0m\x1b[38;2;90;0;39m#\x1b[0m"
	if got != want {
		t.Fatalf("density-based A/B mix:\n got %q\nwant %q", got, want)
	}
}

func TestANSIABDensestGlyphReachesFullColorA(t *testing.T) {
	frame := Frame{Columns: 1, Rows: 1, Cells: []Cell{{Glyph: '█', Opacity: maxOpacity}}}
	got := frame.ANSIAB(RGB{R: 255, G: 255, B: 255}, RGB{}, RGB{R: 40, G: 44, B: 52})
	if !bytes.Contains([]byte(got), []byte("\x1b[38;2;255;255;255m█")) {
		t.Fatalf("densest glyph did not reach full color A: %q", got)
	}
}

func TestANSIABSparseGlyphReachesFullColorBEvenAtFullOpacity(t *testing.T) {
	// Under the old coupled scheme, full opacity implied a pure-colorA mix.
	// The mix now follows ramp density instead, so a sparse glyph stays
	// colorB even when it is fully visible.
	frame := Frame{Columns: 1, Rows: 1, Cells: []Cell{{Glyph: '·', Opacity: maxOpacity}}}
	got := frame.ANSIAB(RGB{R: 255, G: 0, B: 0}, RGB{R: 0, G: 0, B: 255}, RGB{})
	if !bytes.Contains([]byte(got), []byte("\x1b[38;2;0;0;255m")) {
		t.Fatalf("sparse glyph at full opacity did not reach full color B: %q", got)
	}
}

func TestScreenRendererSendsOnlyChanges(t *testing.T) {
	foreground := RGB{R: 74, G: 222, B: 128}
	background := RGB{R: 19, G: 21, B: 31}
	p := New(80, 24)
	frame := p.RenderFastInto(make([]Cell, 0, 80*24))
	renderer := NewScreenRenderer(80, 24, 24)

	initial := renderer.AppendFrame(nil, frame, foreground, background)
	if !bytes.Contains(initial, []byte("\x1b[2J")) {
		t.Fatal("initial update did not clear the screen")
	}
	if unchanged := renderer.AppendFrame(nil, frame, foreground, background); len(unchanged) != 0 {
		t.Fatalf("unchanged frame emitted %d bytes, want 0", len(unchanged))
	}

	p.Step()
	frame = p.RenderFastInto(frame.Cells)
	delta := renderer.AppendFrame(nil, frame, foreground, background)
	full := frame.ANSI(foreground, background)
	t.Logf("80x24 output: initial=%d bytes, delta=%d bytes, full=%d bytes", len(initial), len(delta), len(full))
	if len(delta) >= len(full)/2 {
		t.Fatalf("delta is unexpectedly large: delta=%d full=%d", len(delta), len(full))
	}
}

func TestScreenRendererPreservesDefaultBackground(t *testing.T) {
	p := New(8, 3)
	frame := p.RenderFastInto(make([]Cell, 0, 24))
	renderer := NewScreenRenderer(8, 3, 24)
	got := renderer.AppendFrameDefaultBackground(nil, frame,
		RGB{R: 74, G: 222, B: 128}, RGB{})
	if !bytes.Contains(got, []byte("\x1b[49m")) {
		t.Fatalf("default-background update lacks SGR 49: %q", got)
	}
	if bytes.Contains(got, []byte("\x1b[48;2;")) {
		t.Fatalf("default-background update painted an explicit background: %q", got)
	}
}

func TestQuantizedRendererMatchesConvenienceMethod(t *testing.T) {
	p := New(40, 12)
	frame := p.RenderFastInto(make([]Cell, 0, 40*12))
	foreground := RGB{R: 74, G: 222, B: 128}
	background := RGB{R: 40, G: 44, B: 52}
	want := frame.ANSIQuantized(foreground, background, 24)
	got := NewQuantizedRenderer(24).Render(frame, foreground, background)
	if got != want {
		t.Fatal("stateful quantized renderer differs from convenience method")
	}
}

func TestQuantizedRendererSize(t *testing.T) {
	p := New(160, 50)
	frame := p.RenderFastInto(make([]Cell, 0, 160*50))
	foreground := RGB{R: 74, G: 222, B: 128}
	background := RGB{R: 40, G: 44, B: 52}
	for _, shades := range []int{12, 16, 24} {
		view := NewQuantizedRenderer(shades).Render(frame, foreground, background)
		t.Logf("160x50, %d shades: %d view bytes", shades, len(view))
	}
}

func TestRenderFastIntoReusesStorage(t *testing.T) {
	p := New(80, 24)
	cells := make([]Cell, 0, 80*24)
	frame := p.RenderFastInto(cells)
	cells = frame.Cells
	if allocations := testing.AllocsPerRun(100, func() {
		frame = p.RenderFastInto(cells)
		cells = frame.Cells
	}); allocations != 0 {
		t.Fatalf("RenderFastInto allocated %.2f times per frame", allocations)
	}
}

func findCell(f Frame, column, row int) (Cell, bool) {
	for _, cell := range f.Cells {
		if cell.Column == column && cell.Row == row {
			return cell, true
		}
	}
	return Cell{}, false
}

func BenchmarkRenderFastInto(b *testing.B) {
	p := New(160, 50)
	cells := make([]Cell, 0, 160*50)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame := p.RenderFastInto(cells)
		cells = frame.Cells
		p.Step()
	}
}

func BenchmarkScreenDelta(b *testing.B) {
	p := New(160, 50)
	renderer := NewScreenRenderer(160, 50, 24)
	foreground := RGB{R: 74, G: 222, B: 128}
	background := RGB{R: 19, G: 21, B: 31}
	cells := make([]Cell, 0, 160*50)
	buffer := make([]byte, 0, 160*50*2)
	frame := p.RenderFastInto(cells)
	renderer.AppendFrame(buffer[:0], frame, foreground, background)
	cells = frame.Cells
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Step()
		frame = p.RenderFastInto(cells)
		cells = frame.Cells
		buffer = renderer.AppendFrame(buffer[:0], frame, foreground, background)
	}
}

func BenchmarkQuantizedRenderer(b *testing.B) {
	p := New(160, 50)
	renderer := NewQuantizedRenderer(24)
	foreground := RGB{R: 74, G: 222, B: 128}
	background := RGB{R: 40, G: 44, B: 52}
	cells := make([]Cell, 0, 160*50)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		frame := p.RenderFastInto(cells)
		cells = frame.Cells
		_ = renderer.Render(frame, foreground, background)
		p.Step()
	}
}
