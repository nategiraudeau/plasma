package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// field is a focused Bubble Tea input component: arrow-key stepping when idle,
// cursor-based text editing on Enter, validation on commit, and Escape to undo.
type field struct {
	label, help, value, buffer string
	step                       float64
	signed                     bool
	editing, replace           bool
	cursor                     int
}

func (f *field) start() {
	f.buffer = f.value
	f.cursor = len(f.buffer)
	f.editing = true
	f.replace = true
}
func (f *field) cancel() { f.editing = false; f.buffer = "" }
func (f *field) commit() error {
	if !f.editing {
		return nil
	}
	value := strings.TrimSpace(f.buffer)
	if f.step == 0 {
		if _, err := parseColor(value); err != nil {
			return err
		}
		value = strings.ToLower(value)
	} else {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || (!f.signed && n < 0) {
			if f.signed {
				return fmt.Errorf("%s must be a finite number", f.label)
			}
			return fmt.Errorf("%s must be a finite number ≥ 0", f.label)
		}
		value = strconv.FormatFloat(n, 'f', -1, 64)
	}
	f.value = value
	f.cancel()
	return nil
}
func (f *field) insert(text string) {
	// Fields accept ASCII numbers / hex colors; discard terminal control bytes.
	var clean strings.Builder
	for _, r := range text {
		if r >= 32 && r < 127 {
			clean.WriteRune(r)
		}
	}
	text = clean.String()
	if f.replace {
		f.buffer = ""
		f.cursor = 0
		f.replace = false
	}
	if len(text)+len(f.buffer) > 32 {
		return
	}
	f.buffer = f.buffer[:f.cursor] + text + f.buffer[f.cursor:]
	f.cursor += len(text)
}
func (f *field) update(key tea.KeyPressMsg) error {
	k := key.String()
	if !f.editing {
		switch k {
		case "enter":
			f.start()
		case "left", "right":
			if f.step == 0 {
				return nil
			}
			n, _ := strconv.ParseFloat(f.value, 64)
			if k == "left" {
				n -= f.step
			} else {
				n += f.step
			}
			if math.Abs(n) < 1e300 {
				n = math.Round(n*1e6) / 1e6
			}
			if !f.signed {
				n = math.Max(0, n)
			}
			f.value = strconv.FormatFloat(n, 'f', -1, 64)
		default:
			if key.Text != "" {
				f.start()
				f.insert(key.Text)
			}
		}
		return nil
	}
	switch k {
	case "enter":
		return f.commit()
	case "esc":
		f.cancel()
	case "ctrl+a":
		f.replace = true
	case "left":
		f.cursor = max(0, f.cursor-1)
		f.replace = false
	case "right":
		f.cursor = min(len(f.buffer), f.cursor+1)
		f.replace = false
	case "home":
		f.cursor = 0
		f.replace = false
	case "end":
		f.cursor = len(f.buffer)
		f.replace = false
	case "backspace":
		if f.replace {
			f.buffer = ""
			f.cursor = 0
			f.replace = false
		} else if f.cursor > 0 {
			f.buffer = f.buffer[:f.cursor-1] + f.buffer[f.cursor:]
			f.cursor--
		}
	case "delete":
		if f.replace {
			f.buffer = ""
			f.cursor = 0
			f.replace = false
		} else if f.cursor < len(f.buffer) {
			f.buffer = f.buffer[:f.cursor] + f.buffer[f.cursor+1:]
		}
	default:
		if key.Text != "" {
			f.insert(key.Text)
		}
	}
	return nil
}
func (f field) view(focused bool) string {
	value := f.value
	if f.editing {
		value = f.buffer
		if f.replace {
			return "\x1b[7m" + value + "\x1b[0m"
		}
		left, right := value[:f.cursor], value[f.cursor:]
		cursor := " "
		if len(right) > 0 {
			cursor = right[:1]
			right = right[1:]
		}
		return left + "\x1b[7m" + cursor + "\x1b[0m" + right
	}
	if focused {
		return "\x1b[7m " + value + " \x1b[0m"
	}
	return " " + value + " "
}
