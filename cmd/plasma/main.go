// Command plasma runs the tui.studio plasma as a full-screen Bubble Tea app.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"plasma"
	"plasma/internal/preset"
)

const (
	frameRate     = 60
	paletteShades = 16
)

var (
	defaultColorA color.Color = lipgloss.Color("#4267d6")
	defaultColorB color.Color = lipgloss.Color("#00b3ff")
)

type tickMsg time.Time

type model struct {
	plasma          *plasma.Component
	renderer        *plasma.QuantizedRenderer
	cells           []plasma.Cell
	width           int
	height          int
	colorA          plasma.RGB
	colorB          plasma.RGB
	background      plasma.RGB
	backgroundColor color.Color
	colorReady      bool
	speed           float64
}

func newModel(a, b color.Color) *model {
	return &model{
		plasma:   plasma.New(0, 0),
		renderer: plasma.NewQuantizedRenderer(paletteShades),
		colorA:   rgb(a),
		colorB:   rgb(b),
		speed:    1,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, tick())
}

func (m *model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.BackgroundColorMsg:
		m.backgroundColor = message.Color
		m.background = rgb(message.Color)
		m.colorReady = true
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = message.Width, message.Height
		m.plasma.Resize(m.width, m.height)
		if cap(m.cells) < m.width*m.height {
			m.cells = make([]plasma.Cell, 0, m.width*m.height)
		}
		return m, nil
	case tickMsg:
		if m.colorReady {
			m.plasma.SetTime(m.plasma.Time() + plasma.FrameStep*m.speed)
		}
		return m, tick()
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *model) View() tea.View {
	view := tea.NewView("")
	view.AltScreen = true
	view.WindowTitle = "Plasma"
	if !m.colorReady || m.width == 0 || m.height == 0 {
		return view
	}
	frame := m.plasma.RenderFastInto(m.cells)
	m.cells = frame.Cells
	view.SetContent(m.renderer.RenderAB(frame, m.colorA, m.colorB, m.background))
	view.BackgroundColor = m.backgroundColor
	return view
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/frameRate, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func rgb(value color.Color) plasma.RGB {
	red, green, blue, _ := value.RGBA()
	return plasma.RGB{R: uint8(red >> 8), G: uint8(green >> 8), B: uint8(blue >> 8)}
}

// modelFromArgs loads the same validated format as paint. Explicit flags
// override saved values; running without a file retains the original demo.
func modelFromArgs(args []string) (*model, error) {
	flags := flag.NewFlagSet("plasma", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprintln(flags.Output(), "Usage: plasma [flags] [mask.json]"); flags.PrintDefaults() }
	colorAFlag := flags.String("a", "#4267d6", "primary plasma color (#RRGGBB)")
	colorBFlag := flags.String("b", "#00b3ff", "color approached while fading (#RRGGBB)")
	rotationFlag := flags.Float64("rotation", 0, "clockwise wave rotation in degrees")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() > 1 {
		return nil, fmt.Errorf("usage: plasma [flags] [mask.json]")
	}
	var saved *preset.Document
	if flags.NArg() == 1 {
		d, fresh, err := preset.Load(flags.Arg(0))
		if err != nil {
			return nil, fmt.Errorf("load mask: %w", err)
		}
		if fresh {
			return nil, fmt.Errorf("mask file %q does not exist", flags.Arg(0))
		}
		saved = &d
		explicit := map[string]bool{}
		flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		if !explicit["a"] {
			*colorAFlag = d.ColorA
		}
		if !explicit["b"] {
			*colorBFlag = d.ColorB
		}
		if !explicit["rotation"] {
			*rotationFlag = d.Rotation
		}
	}
	a, err := parseColor(*colorAFlag)
	if err != nil {
		return nil, fmt.Errorf("-a: %w", err)
	}
	b, err := parseColor(*colorBFlag)
	if err != nil {
		return nil, fmt.Errorf("-b: %w", err)
	}
	m := newModel(a, b)
	if saved != nil {
		if err := m.plasma.SetMask(saved.Mask()); err != nil {
			return nil, err
		}
		m.speed = saved.Speed
	}
	if err := m.plasma.SetRotation(*rotationFlag); err != nil {
		return nil, fmt.Errorf("-rotation: %w", err)
	}
	return m, nil
}

func main() {
	m, err := modelFromArgs(os.Args[1:])
	if err == flag.ErrHelp {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "plasma:", err)
		os.Exit(2)
	}
	if _, err = tea.NewProgram(m, tea.WithFPS(frameRate)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "plasma:", err)
		os.Exit(1)
	}
}

func parseColor(value string) (color.Color, error) {
	hex := strings.TrimPrefix(value, "#")
	if len(hex) != 6 {
		return nil, fmt.Errorf("expected #RRGGBB, got %q", value)
	}
	if _, err := strconv.ParseUint(hex, 16, 24); err != nil {
		return nil, fmt.Errorf("expected #RRGGBB, got %q", value)
	}
	return lipgloss.Color("#" + hex), nil
}
