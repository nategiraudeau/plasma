package main

import (
	"image/color"
	"os"
	"path/filepath"
	"plasma"
	"plasma/internal/preset"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestBackgroundColorMessageDrivesBlendColor(t *testing.T) {
	m := newModel(defaultColorA, defaultColorB)
	message := tea.BackgroundColorMsg{Color: color.RGBA{R: 0x28, G: 0x2c, B: 0x34, A: 0xff}}
	updated, _ := m.Update(message)
	got := updated.(*model)
	if !got.colorReady {
		t.Fatal("background color message did not mark color ready")
	}
	if got.background.R != 0x28 || got.background.G != 0x2c || got.background.B != 0x34 {
		t.Fatalf("blend background = %+v, want #282c34", got.background)
	}
	if got.backgroundColor != message.Color {
		t.Fatal("view background did not retain the detected terminal color")
	}
}

func TestWindowSizeResizesPlasma(t *testing.T) {
	m := newModel(defaultColorA, defaultColorB)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	got := updated.(*model)
	if got.width != 100 || got.height != 30 || cap(got.cells) < 3000 {
		t.Fatalf("resize state = %dx%d, cell capacity %d", got.width, got.height, cap(got.cells))
	}
}

func BenchmarkModelView160x50(b *testing.B) {
	m := newModel(defaultColorA, defaultColorB)
	m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 0x28, G: 0x2c, B: 0x34, A: 0xff}})
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.plasma.Step()
		_ = m.View()
	}
}

func TestParseColor(t *testing.T) {
	got, err := parseColor("#282c34")
	if err != nil {
		t.Fatal(err)
	}
	if value := rgb(got); value.R != 0x28 || value.G != 0x2c || value.B != 0x34 {
		t.Fatalf("parsed color = %+v, want #282c34", value)
	}
	if _, err := parseColor("blue"); err == nil {
		t.Fatal("invalid color was accepted")
	}
}

func TestSavedMaskPlaybackAndFlagOverrides(t *testing.T) {
	d := preset.New()
	d.Contours = [][]preset.Point{{{X: .3, Y: .2}, {X: 1, Y: .2}, {X: 1, Y: .8}, {X: .3, Y: .8}}}
	d.Edges.Right = []plasma.EdgeSpan{{Start: .2, End: .8}}
	d.Feather = .04
	d.Distortion = .5
	d.Rotation = -30
	d.Speed = 2
	d.ColorA = "#ff1100"
	d.ColorB = "#00ff22"
	name := filepath.Join(t.TempDir(), "mask.json")
	if err := preset.Save(name, d); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(name)
	m, err := modelFromArgs([]string{name})
	if err != nil {
		t.Fatal(err)
	}
	if m.speed != 2 || m.plasma.Rotation() != 330 || m.colorA != (plasma.RGB{R: 255, G: 17}) || m.colorB != (plasma.RGB{G: 255, B: 34}) {
		t.Fatal("saved animation settings weren't applied")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	want := plasma.New(100, 30)
	want.SetMask(d.Mask())
	want.SetRotation(d.Rotation)
	if !reflect.DeepEqual(m.plasma.Render(), want.Render()) {
		t.Fatal("saved mask and edge extensions weren't applied")
	}
	m.colorReady = true
	m.Update(tickMsg{})
	if m.plasma.Time() != 2*plasma.FrameStep {
		t.Fatal("saved speed wasn't applied")
	}
	override, err := modelFromArgs([]string{"-rotation", "0", "-a", "#123456", name})
	if err != nil {
		t.Fatal(err)
	}
	if override.plasma.Rotation() != 0 || override.colorA != (plasma.RGB{R: 18, G: 52, B: 86}) || override.colorB != m.colorB {
		t.Fatal("explicit flags should override only selected saved settings")
	}
	after, _ := os.ReadFile(name)
	if string(before) != string(after) {
		t.Fatal("playback modified the saved file")
	}
}
func TestPlayerDefaultsMissingAndInvalidFiles(t *testing.T) {
	m, err := modelFromArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	m.plasma.Resize(80, 24)
	if m.speed != 1 || !reflect.DeepEqual(m.plasma.Render(), plasma.New(80, 24).Render()) {
		t.Fatal("original no-file demo changed")
	}
	name := filepath.Join(t.TempDir(), "missing.json")
	if _, err = modelFromArgs([]string{name}); err == nil {
		t.Fatal("missing file accepted")
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("player created a missing file")
	}
	if err = os.WriteFile(name, []byte(`{"version":999}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = modelFromArgs([]string{name}); err == nil {
		t.Fatal("invalid file accepted")
	}
	if _, err = modelFromArgs([]string{"one", "two"}); err == nil {
		t.Fatal("extra positional arguments accepted")
	}
}
