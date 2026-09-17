// Command paint edits a plasma mask in a full-screen terminal.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"plasma"
)

const (
	canvasScreen = iota
	configScreen
	previewScreen
)
const footerRows = 3

type tickMsg time.Time

type model struct {
	filename                            string
	doc                                 document
	canvas                              canvas
	plasma                              *plasma.Component
	renderer                            *plasma.QuantizedRenderer
	cells                               []plasma.Cell
	background                          plasma.RGB
	width, height, screen, selected     int
	fields                              []field
	erase, dragging, dirty, confirmExit bool
	edgeBrush                           bool
	last                                point
	status                              string
}

func newModel(filename string, doc document, fresh bool) *model {
	m := &model{filename: filename, doc: doc, canvas: newCanvas(doc.Contours), plasma: plasma.New(0, 0), renderer: plasma.NewQuantizedRenderer(24), dirty: fresh}
	m.canvas.loadEdges(doc.Edges)
	m.fields = []field{
		{label: "Feather", help: "Inward edge width · fraction of region", value: strconv.FormatFloat(doc.Feather, 'f', -1, 64), step: .01},
		{label: "Distortion", help: "Edge motion · 0 stops the mask", value: strconv.FormatFloat(doc.Distortion, 'f', -1, 64), step: .1},
		{label: "Speed", help: "Plasma animation speed · 0 pauses", value: strconv.FormatFloat(doc.Speed, 'f', -1, 64), step: .1},
		{label: "Color A", help: "Dense glyph color · #RRGGBB", value: doc.ColorA},
		{label: "Color B", help: "Sparse glyph color · #RRGGBB", value: doc.ColorB},
		{label: "Rotation", help: "Wave direction · clockwise degrees", value: strconv.FormatFloat(doc.Rotation, 'f', -1, 64), step: 5, signed: true},
	}
	m.plasma.SetMask(doc.Mask())
	m.plasma.SetRotation(doc.Rotation)
	return m
}
func (m *model) Init() tea.Cmd     { return tea.Batch(tea.RequestBackgroundColor, tick()) }
func tick() tea.Cmd                { return tea.Tick(time.Second/60, func(t time.Time) tea.Msg { return tickMsg(t) }) }
func (m *model) canvasHeight() int { return max(0, m.height-footerRows) }
func (m *model) drawable() bool    { return m.width >= 48 && m.height >= 16 }

func (m *model) syncFields() {
	old := m.doc
	m.doc.Feather, _ = strconv.ParseFloat(m.fields[0].value, 64)
	m.doc.Distortion, _ = strconv.ParseFloat(m.fields[1].value, 64)
	m.doc.Speed, _ = strconv.ParseFloat(m.fields[2].value, 64)
	m.doc.ColorA, m.doc.ColorB = m.fields[3].value, m.fields[4].value
	m.doc.Rotation, _ = strconv.ParseFloat(m.fields[5].value, 64)
	if old.Feather != m.doc.Feather || old.Distortion != m.doc.Distortion || old.Speed != m.doc.Speed || old.ColorA != m.doc.ColorA || old.ColorB != m.doc.ColorB || old.Rotation != m.doc.Rotation {
		if old.Rotation != m.doc.Rotation {
			m.plasma.SetRotation(m.doc.Rotation)
		}
		m.dirty = true
		m.status = ""
		if old.Feather != m.doc.Feather || old.Distortion != m.doc.Distortion {
			m.plasma.SetMask(m.doc.Mask())
		}
	}
}
func (m *model) commitField() bool {
	if err := m.fields[m.selected].commit(); err != nil {
		m.status = err.Error()
		return false
	}
	m.syncFields()
	return true
}
func (m *model) finishStroke() {
	if !m.dragging {
		return
	}
	m.dragging = false
	m.doc.Contours = m.canvas.contours()
	m.doc.Edges = m.canvas.extensions()
	m.plasma.SetMask(m.doc.Mask())
	m.dirty = true
	m.status = ""
}
func (m *model) save() bool {
	m.finishStroke()
	if !m.commitField() {
		m.screen = configScreen
		return false
	}
	if err := saveDocument(m.filename, m.doc); err != nil {
		m.status = "Save failed: " + err.Error()
		return false
	}
	m.dirty = false
	m.status = "Saved " + filepath.Base(m.filename)
	return true
}
func (m *model) mousePoint(x, y int) point {
	return point{X: (float64(x) + .5) / float64(m.width), Y: (float64(y) + .5) / float64(m.canvasHeight())}
}
func (m *model) paintTo(x, y int) {
	// Clamp to the canvas edge when a drag leaves it; never paint the footer.
	x = max(0, min(m.width-1, x))
	y = max(0, min(m.canvasHeight()-1, y))
	p := m.mousePoint(x, y)
	if m.erase || !m.edgeBrush {
		m.canvas.stroke(m.last, p, m.doc.Brush, m.width, m.canvasHeight(), m.erase)
	}
	if m.erase || m.edgeBrush {
		m.canvas.edgeStroke(m.last, p, m.doc.Brush, m.width, m.canvasHeight(), m.erase)
	}
	m.last = p
}
func (m *model) requestExit() tea.Cmd {
	m.finishStroke()
	if !m.commitField() {
		m.screen = configScreen
		return nil
	}
	if !m.dirty {
		return tea.Quit
	}
	m.confirmExit = true
	if m.screen == previewScreen {
		m.screen = canvasScreen
	}
	return nil
}
func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.finishStroke()
		m.width, m.height = msg.Width, msg.Height
		m.plasma.Resize(m.width, m.height)
	case tea.BackgroundColorMsg:
		r, g, b, _ := msg.Color.RGBA()
		m.background = plasma.RGB{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8)}
	case tickMsg:
		if m.screen == previewScreen && !m.confirmExit {
			m.plasma.SetTime(m.plasma.Time() + plasma.FrameStep*m.doc.Speed)
		}
		return m, tick()
	case tea.BlurMsg:
		m.finishStroke()
	case tea.MouseClickMsg:
		if m.confirmExit || !m.drawable() {
			break
		}
		if m.screen == canvasScreen && msg.Button == tea.MouseLeft && msg.Y >= 0 && msg.Y < m.canvasHeight() && msg.X >= 0 && msg.X < m.width {
			m.dragging = true
			m.last = m.mousePoint(msg.X, msg.Y)
			m.paintTo(msg.X, msg.Y)
		} else if m.screen == configScreen && msg.Button == tea.MouseLeft {
			row := (msg.Y - 2) / m.fieldSpacing()
			if msg.Y >= 2 && row < len(m.fields) && m.commitField() {
				m.selected = row
				m.fields[row].start()
			}
		}
	case tea.MouseMotionMsg:
		if m.dragging && m.screen == canvasScreen {
			m.paintTo(msg.X, msg.Y)
		}
	case tea.MouseReleaseMsg:
		if m.dragging {
			m.paintTo(msg.X, msg.Y)
			m.finishStroke()
		}
	case tea.MouseWheelMsg:
		if !m.confirmExit && m.screen == canvasScreen {
			m.finishStroke()
			delta := 2
			if msg.Button == tea.MouseWheelDown {
				delta = -2
			}
			m.doc.Brush = max(1, min(31, m.doc.Brush+delta))
			m.dirty = true
		}
	case tea.PasteMsg:
		if !m.confirmExit && m.screen == configScreen {
			f := &m.fields[m.selected]
			if !f.editing {
				f.start()
			}
			f.insert(msg.Content)
		}
	case tea.KeyPressMsg:
		key := msg.String()
		if m.confirmExit {
			switch key {
			case "y", "Y":
				if m.save() {
					return m, tea.Quit
				}
				m.confirmExit = false
			case "n", "N":
				return m, tea.Quit
			case "esc":
				m.confirmExit = false
			}
			return m, nil
		}
		switch key {
		case "ctrl+c", "ctrl+x":
			return m, m.requestExit()
		case "ctrl+s":
			saved := m.save()
			if !saved && m.screen == previewScreen {
				m.screen = canvasScreen
			}
			return m, nil
		case "tab", "shift+tab":
			m.finishStroke()
			if !m.commitField() {
				m.screen = configScreen
				return m, nil
			}
			delta := 1
			if key == "shift+tab" {
				delta = 2
			}
			m.screen = (m.screen + delta) % 3
			m.status = ""
			return m, nil
		}
		switch m.screen {
		case canvasScreen:
			switch key {
			case "b":
				m.finishStroke()
				m.edgeBrush = !m.edgeBrush
				m.erase = false
			case "e":
				m.finishStroke()
				m.erase = !m.erase
			case "[", "-":
				m.finishStroke()
				m.doc.Brush = max(1, m.doc.Brush-2)
				m.dirty = true
			case "]", "+", "=":
				m.finishStroke()
				m.doc.Brush = min(31, m.doc.Brush+2)
				m.dirty = true
			case "c":
				m.dragging = false
				m.canvas = newCanvas(nil)
				m.doc.Contours = [][]point{}
				m.doc.Edges = plasma.EdgeExtensions{}
				m.plasma.SetMask(m.doc.Mask())
				m.dirty = true
				m.status = "Cleared"
			}
		case configScreen:
			if key == "up" || key == "down" {
				if m.commitField() {
					delta := 1
					if key == "up" {
						delta = len(m.fields) - 1
					}
					m.selected = (m.selected + delta) % len(m.fields)
					m.status = ""
				}
			} else {
				if err := m.fields[m.selected].update(msg); err != nil {
					m.status = err.Error()
				} else {
					m.status = ""
				}
				m.syncFields()
			}
		case previewScreen:
			if key == "esc" {
				m.screen = canvasScreen
			}
		}
	}
	return m, nil
}

func fit(text string, width int) string {
	text = ansi.Truncate(text, max(0, width), "")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}
func safeText(text string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, ansi.Strip(text))
}
func (m *model) footer() []string {
	title := "Canvas · Paint"
	if m.edgeBrush {
		title = "Canvas · Edge brush (red)"
	}
	if m.erase {
		title = "Canvas · Erase"
	}
	title += fmt.Sprintf(" · Brush %d", m.doc.Brush)
	first := "Press -: smaller brush  Press +: larger brush"
	brushAction, eraseAction := "B Edge", "E Erase"
	if m.edgeBrush {
		brushAction = "B Paint"
	}
	if m.erase {
		eraseAction = "E Paint"
		if m.edgeBrush {
			eraseAction = "E Edge"
		}
	}
	second := "Tab Next " + brushAction + " " + eraseAction + " C Clear ^S Save ^X Exit"
	if m.screen == configScreen {
		title = "Configuration"
		first, second = "Tab Preview   ↑↓ Select   ←→ Adjust", "Enter Edit/Apply   Esc Cancel   ^S Save   ^X Exit"
		if m.fields[m.selected].editing {
			first, second = "←→ Cursor   Home/End   ^A Select all", "Enter Apply   Esc Cancel   ^S Save   ^X Exit"
		}
	}
	title += " · " + safeText(filepath.Base(m.filename))
	if m.dirty {
		title += " *"
	}
	if m.status != "" {
		title = safeText(m.status)
	}
	if m.confirmExit {
		title = "Save changes?"
		first = "Y Save and exit   N Discard and exit"
		second = "Esc Cancel"
	}
	return []string{fit(title, m.width), "\x1b[7m" + fit(first, m.width) + "\x1b[0m", "\x1b[7m" + fit(second, m.width) + "\x1b[0m"}
}
func (m *model) canvasView() []string {
	lines := make([]string, 0, m.height)
	for y := 0; y < m.canvasHeight(); y++ {
		var line strings.Builder
		colors := []string{"0;0;0", "255;255;255", "255;0;0"}
		previousFG, previousBG := -1, -1
		for x := 0; x < m.width; x++ {
			upper, lower := m.canvasTone(x, y, false), m.canvasTone(x, y, true)
			fg, bg, glyph := upper, lower, '▀'
			if upper == lower {
				bg = 0
				glyph = '█'
				if upper == 0 {
					glyph = ' '
				}
			}
			if fg != previousFG {
				line.WriteString("\x1b[38;2;" + colors[fg] + "m")
				previousFG = fg
			}
			if bg != previousBG {
				line.WriteString("\x1b[48;2;" + colors[bg] + "m")
				previousBG = bg
			}
			line.WriteRune(glyph)
		}
		line.WriteString("\x1b[0m")
		lines = append(lines, line.String())
	}
	return lines
}
func (m *model) fieldSpacing() int {
	if m.canvasHeight() >= 2*len(m.fields)+2 {
		return 2
	}
	return 1
}

func (m *model) configView() []string {
	lines := make([]string, m.canvasHeight())
	lines[0] = "  Configuration"
	for i, f := range m.fields {
		mark := "  "
		if i == m.selected {
			mark = "› "
		}
		lines[2+i*m.fieldSpacing()] = fmt.Sprintf("  %s%-12s %s", mark, f.label, f.view(i == m.selected))
	}
	lines[len(lines)-1] = "  " + m.fields[m.selected].help
	for i := range lines {
		lines[i] = fit(lines[i], m.width)
	}
	return lines
}
func (m *model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.WindowTitle = "Paint · " + safeText(filepath.Base(m.filename))
	view.ReportFocus = true
	if m.width <= 0 || m.height <= 0 {
		return view
	}
	if !m.drawable() {
		lines := make([]string, m.height)
		lines[0] = fit("Resize terminal to at least 48 × 16", m.width)
		if m.height > 1 {
			lines[m.height-1] = fit("^S Save  ^X Exit", m.width)
		}
		if m.confirmExit {
			lines[0] = fit("Save? Y yes / N discard / Esc cancel", m.width)
		}
		view.SetContent(strings.Join(lines, "\n"))
		return view
	}
	if m.screen == previewScreen {
		a, _ := parseColor(m.doc.ColorA)
		b, _ := parseColor(m.doc.ColorB)
		frame := m.plasma.RenderFastInto(m.cells)
		m.cells = frame.Cells
		view.SetContent(m.renderer.RenderAB(frame, a, b, m.background))
		return view
	}
	view.MouseMode = tea.MouseModeCellMotion
	var lines []string
	if m.screen == canvasScreen {
		lines = m.canvasView()
	} else {
		lines = m.configView()
	}
	lines = append(lines, m.footer()...)
	view.SetContent(strings.Join(lines, "\n"))
	return view
}
func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: paint FILE")
	}
	doc, fresh, err := loadDocument(args[0])
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(newModel(args[0], doc, fresh), tea.WithFPS(60)).Run()
	return err
}
func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "paint:", err)
		os.Exit(1)
	}
}
