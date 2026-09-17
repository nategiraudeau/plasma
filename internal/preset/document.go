// Package preset shares the saved paint format between the editor and player.
package preset

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"plasma"
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Document stores normalized vector geometry and the animation settings.
type Document struct {
	Version    int                   `json:"version"`
	Contours   [][]Point             `json:"contours"`
	Edges      plasma.EdgeExtensions `json:"edges,omitzero"`
	Feather    float64               `json:"feather"`
	Distortion float64               `json:"distortion"`
	Speed      float64               `json:"speed"`
	Rotation   float64               `json:"rotation"`
	ColorA     string                `json:"color_a"`
	ColorB     string                `json:"color_b"`
	Brush      int                   `json:"brush"`
}

func New() Document {
	defaults := plasma.DefaultMask()
	return Document{Version: 1, Contours: [][]Point{}, Feather: defaults.Feather, Distortion: defaults.Distortion, Speed: 1, ColorA: "#4267d6", ColorB: "#00b3ff", Brush: 5}
}

func (d Document) Mask() plasma.Mask {
	path := new(plasma.Path)
	for _, c := range d.Contours {
		if len(c) == 0 {
			continue
		}
		path.MoveTo(c[0].X, c[0].Y)
		for _, p := range c[1:] {
			path.LineTo(p.X, p.Y)
		}
	}
	return plasma.Mask{Path: path, Feather: d.Feather, Distortion: d.Distortion, Extensions: d.Edges}
}

func (d Document) Validate() error {
	if d.Version != 1 {
		return fmt.Errorf("unsupported paint file version %d", d.Version)
	}
	for _, v := range []float64{d.Feather, d.Distortion, d.Speed} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("feather, distortion and speed must be finite and nonnegative")
		}
	}
	if math.IsNaN(d.Rotation) || math.IsInf(d.Rotation, 0) {
		return fmt.Errorf("rotation must be finite")
	}
	if d.Brush < 1 || d.Brush > 31 {
		return fmt.Errorf("brush must be between 1 and 31")
	}
	if _, err := ParseColor(d.ColorA); err != nil {
		return err
	}
	if _, err := ParseColor(d.ColorB); err != nil {
		return err
	}
	count := 0
	for _, c := range d.Contours {
		if len(c) < 3 {
			return fmt.Errorf("each contour must have at least three points")
		}
		count += len(c)
		for _, p := range c {
			if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) || p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 {
				return fmt.Errorf("path coordinates must be between 0 and 1")
			}
		}
	}
	if count > 100000 {
		return fmt.Errorf("mask exceeds 100000 points")
	}
	// Share the renderer's validation for normalized edge spans.
	return plasma.New(0, 0).SetMask(d.Mask())
}

func Load(name string) (Document, bool, error) {
	d := New()
	f, err := os.Open(name)
	if os.IsNotExist(err) {
		return d, true, nil
	}
	if err != nil {
		return d, false, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 8<<20))
	decoder.DisallowUnknownFields()
	decoded := &d
	if err = decoder.Decode(&decoded); err != nil {
		return d, false, fmt.Errorf("read paint file: %w", err)
	}
	if decoded == nil {
		return d, false, fmt.Errorf("paint file must be a JSON object")
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return d, false, fmt.Errorf("paint file must contain one JSON object")
	}
	return d, false, d.Validate()
}

// Write beside the destination, then rename: interrupted writes do not truncate
// the user's previous Mask. Preserve an existing file's permissions.
func Save(name string, d Document) error {
	if err := d.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(0644)
	if info, err := os.Lstat(name); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("destination must be a regular file")
		}
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".paint-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), name)
}

func ParseColor(s string) (plasma.RGB, error) {
	if len(s) != 7 || s[0] != '#' {
		return plasma.RGB{}, fmt.Errorf("color must be #RRGGBB")
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 24)
	if err != nil {
		return plasma.RGB{}, fmt.Errorf("color must be #RRGGBB")
	}
	return plasma.RGB{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v)}, nil
}
