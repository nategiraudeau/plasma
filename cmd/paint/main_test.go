package main

import (
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func press(m *model, code rune)      { m.Update(tea.KeyPressMsg{Code: code}) }
func typeText(m *model, text string) { m.Update(tea.KeyPressMsg{Code: rune(text[0]), Text: text}) }
func ctrl(m *model, code rune) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl})
	return cmd
}
func testModel(t *testing.T) *model {
	t.Helper()
	m := newModel(filepath.Join(t.TempDir(), "mask.json"), newDocument(), true)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 40, G: 42, B: 50, A: 255}})
	return m
}
func draw(m *model) {
	m.Update(tea.MouseClickMsg{X: 20, Y: 10, Button: tea.MouseLeft})
	m.Update(tea.MouseMotionMsg{X: 70, Y: 10, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 70, Y: 10, Button: tea.MouseLeft})
}

func TestScreensKeepEditsAndResize(t *testing.T) {
	m := testModel(t)
	draw(m)
	if len(m.doc.Contours) != 1 {
		t.Fatal("release did not vectorize")
	}
	original := m.doc.Contours
	press(m, tea.KeyTab)
	if m.screen != configScreen {
		t.Fatal("tab did not enter editor")
	}
	press(m, tea.KeyLeft)
	if m.doc.Feather != .23 {
		t.Fatal("left arrow did not update feather")
	}
	press(m, tea.KeyEnter)
	typeText(m, "0.01")
	press(m, tea.KeyEnter)
	if m.doc.Feather != .01 {
		t.Fatal("typing did not update feather")
	}
	press(m, tea.KeyTab)
	if m.screen != previewScreen || len(m.plasma.Render().Cells) == 0 {
		t.Fatal("preview not using painted mask")
	}
	m.Update(tickMsg{})
	if m.plasma.Time() == 0 {
		t.Fatal("preview isn't animated")
	}
	press(m, tea.KeyTab)
	if m.screen != canvasScreen || !reflect.DeepEqual(original, m.doc.Contours) {
		t.Fatal("tab discarded painting")
	}
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 45})
	if !reflect.DeepEqual(original, m.doc.Contours) || !m.canvas.sample(.4, 10.5/27) {
		t.Fatal("resize discarded or moved normalized geometry")
	}
	ctrl(m, 's')
	saved, fresh, err := loadDocument(m.filename)
	if err != nil || fresh || !reflect.DeepEqual(saved, m.doc) {
		t.Fatalf("save/reopen mismatch: %v", err)
	}
	reopened := newModel(m.filename, saved, false)
	if !reflect.DeepEqual(reopened.canvas.pixels, m.canvas.pixels) {
		t.Fatal("reopened drawing differs")
	}
}

func TestEraseClearAndBrushControls(t *testing.T) {
	m := testModel(t)
	draw(m)
	press(m, ']')
	if m.doc.Brush != 7 {
		t.Fatal("brush increase failed")
	}
	press(m, '[')
	if m.doc.Brush != 5 {
		t.Fatal("brush decrease failed")
	}
	press(m, 'e')
	if !m.erase {
		t.Fatal("erase toggle failed")
	}
	m.Update(tea.MouseClickMsg{X: 40, Y: 10, Button: tea.MouseLeft})
	m.Update(tea.MouseReleaseMsg{X: 40, Y: 10, Button: tea.MouseLeft})
	if m.canvas.sample(40.5/100, 10.5/27) {
		t.Fatal("mouse eraser failed")
	}
	press(m, 'c')
	if len(m.doc.Contours) != 0 || len(m.plasma.Render().Cells) != 0 {
		t.Fatal("clear didn't clear mask and preview")
	}
	ctrl(m, 's')
	d, _, err := loadDocument(m.filename)
	if err != nil || len(d.Contours) != 0 {
		t.Fatal("empty mask failed to persist")
	}
}

func TestInvalidEditorInputAndPaste(t *testing.T) {
	m := testModel(t)
	press(m, tea.KeyTab)
	press(m, tea.KeyEnter)
	typeText(m, "-1")
	press(m, tea.KeyTab)
	if m.screen != configScreen || m.doc.Feather != .24 || m.status == "" {
		t.Fatal("invalid value should retain editor and previous config")
	}
	press(m, tea.KeyEscape)
	if m.fields[0].editing {
		t.Fatal("escape didn't cancel edit")
	}
	for i := 0; i < 3; i++ {
		press(m, tea.KeyDown)
	}
	press(m, tea.KeyEnter)
	m.Update(tea.PasteMsg{Content: "#aabbcc"})
	press(m, tea.KeyEnter)
	if m.doc.ColorA != "#aabbcc" {
		t.Fatal("color paste did not commit")
	}
	press(m, tea.KeyEnter)
	typeText(m, "oops")
	ctrl(m, 's')
	if _, err := os.Stat(m.filename); !os.IsNotExist(err) {
		t.Fatal("invalid editing buffer got saved")
	}
}

func TestSaveFailuresAndExitProtection(t *testing.T) {
	m := testModel(t)
	draw(m)
	ctrl(m, 'x')
	if !m.confirmExit {
		t.Fatal("dirty exit needs choice")
	}
	press(m, tea.KeyEscape)
	if m.confirmExit {
		t.Fatal("cancel exit failed")
	}
	ctrl(m, 'x')
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil || m.dirty {
		t.Fatal("save and exit failed")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("save and exit didn't quit")
	}
	if _, err := os.Stat(m.filename); err != nil {
		t.Fatal(err)
	}
	m.filename = filepath.Join(t.TempDir(), "missing", "mask.json")
	m.dirty = true
	ctrl(m, 'x')
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd != nil || !m.dirty || !strings.Contains(m.status, "Save failed") {
		t.Fatal("failed save must leave app open with edits")
	}
	ctrl(m, 'x')
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd == nil {
		t.Fatal("discard exit failed")
	}
}

func TestViewDimensionsBackgroundAndChrome(t *testing.T) {
	m := testModel(t)
	for _, size := range [][2]int{{48, 16}, {80, 24}, {160, 50}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for screen := 0; screen < 3; screen++ {
			m.screen = screen
			v := m.View()
			if !v.AltScreen || v.BackgroundColor != nil {
				t.Fatal("view must use alternate screen and preserve terminal default")
			}
			lines := strings.Split(v.Content, "\n")
			if len(lines) != m.height {
				t.Fatalf("screen %d has %d rows, want %d", screen, len(lines), m.height)
			}
			for _, line := range lines {
				if ansi.StringWidth(line) != m.width {
					t.Fatalf("screen %d line width=%d want %d: %q", screen, ansi.StringWidth(line), m.width, line)
				}
			}
			if screen == canvasScreen && !strings.Contains(v.Content, "48;2;0;0;0m") {
				t.Fatal("canvas isn't black")
			}
			if screen != canvasScreen && strings.Contains(v.Content, "48;2;0;0;0m") {
				t.Fatal("black canvas background leaked")
			}
			if screen == previewScreen && strings.Contains(v.Content, "^S Save") {
				t.Fatal("preview has chrome")
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 3})
	m.screen = configScreen
	if !strings.Contains(m.View().Content, "Resize") {
		t.Fatal("small terminal needs resize state")
	}
}

func TestPreviewWorksWithoutOSCResponse(t *testing.T) {
	m := newModel("mask.json", newDocument(), false)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.screen = previewScreen
	if len(strings.Split(m.View().Content, "\n")) != 24 {
		t.Fatal("preview blocked waiting for terminal query")
	}
}

func TestDocumentErrorsAndAtomicSave(t *testing.T) {
	name := filepath.Join(t.TempDir(), "mask.json")
	d, fresh, err := loadDocument(name)
	if err != nil || !fresh {
		t.Fatal("new file failed")
	}
	if err = saveDocument(name, d); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(name)
	bad := d
	bad.Feather = -1
	if saveDocument(name, bad) == nil {
		t.Fatal("invalid document saved")
	}
	after, _ := os.ReadFile(name)
	if string(before) != string(after) {
		t.Fatal("failed save changed old file")
	}
	for _, content := range []string{"null", "{} {}", `{"version":999}`, `{"color_a":"red"}`, `{"unknown":true}`, `{"contours":[[{"x":2,"y":0}]]}`} {
		if err = os.WriteFile(name, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err = loadDocument(name); err == nil {
			t.Fatalf("bad document accepted: %s", content)
		}
	}
	if run(nil) == nil || run([]string{"one", "two"}) == nil {
		t.Fatal("must accept exactly one input file")
	}
}

func BenchmarkPaintPreview(b *testing.B) {
	d := newDocument()
	d.Feather = .02
	c := newCanvas(nil)
	for i := 0; i < 12; i++ {
		y := .1 + float64(i)*.07
		c.stroke(point{X: .1, Y: y}, point{X: .9, Y: y + .03}, 3, 160, 47, false)
	}
	d.Contours = c.contours()
	m := newModel("mask.json", d, false)
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m.screen = previewScreen
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.plasma.Step()
		m.View()
	}
}

func TestRotationEditorPersistsWithoutChangingPaint(t *testing.T) {
	m := testModel(t)
	draw(m)
	contours := m.doc.Contours
	m.doc.Feather = .01
	m.fields[0].value = "0.01"
	m.plasma.SetMask(m.doc.Mask())
	before := m.plasma.Render()
	press(m, tea.KeyTab)
	for i := 0; i < 5; i++ {
		press(m, tea.KeyDown)
	}
	press(m, tea.KeyLeft)
	if m.doc.Rotation != -5 || m.plasma.Rotation() != 355 {
		t.Fatal("rotation arrow must allow negative angles")
	}
	press(m, tea.KeyEnter)
	typeText(m, "-32.5")
	press(m, tea.KeyEnter)
	if m.doc.Rotation != -32.5 || m.plasma.Rotation() != 327.5 {
		t.Fatal("exact rotation edit failed")
	}
	press(m, tea.KeyTab)
	if reflect.DeepEqual(before, m.plasma.Render()) {
		t.Fatal("preview didn't rotate")
	}
	ctrl(m, 's')
	d, _, err := loadDocument(m.filename)
	if err != nil || d.Rotation != -32.5 {
		t.Fatalf("rotation not saved: %v", err)
	}
	reopened := newModel(m.filename, d, false)
	if reopened.plasma.Rotation() != 327.5 {
		t.Fatal("saved rotation wasn't applied")
	}
	if !reflect.DeepEqual(contours, m.doc.Contours) {
		t.Fatal("rotation destructively changed drawing")
	}
	// The compact editor must keep all six controls visible at minimum height.
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 16})
	m.screen = configScreen
	if !strings.Contains(m.View().Content, "Rotation") {
		t.Fatal("rotation hidden in minimum-size editor")
	}
}

func TestOlderPaintFileDefaultsToZeroRotation(t *testing.T) {
	name := filepath.Join(t.TempDir(), "old.json")
	if err := os.WriteFile(name, []byte(`{"version":1,"contours":[],"feather":0.24,"distortion":1,"speed":1,"color_a":"#4267d6","color_b":"#00b3ff","brush":5}`), 0600); err != nil {
		t.Fatal(err)
	}
	d, _, err := loadDocument(name)
	if err != nil || d.Rotation != 0 {
		t.Fatalf("old file must load with zero rotation: %v", err)
	}
}
