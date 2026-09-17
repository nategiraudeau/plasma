# plasma

`plasma` is a dependency-free Go component implementing the ASCII plasma from
the background of [tui.studio](https://tui.studio/). The density ramp, wave
formula, grid geometry, and alpha formatting mirror the page source, with a
30% faster fixed time step. A full-height horizontal mask confines the effect to roughly 42–107% of
the screen width, so the right fade extends beyond the viewport and is clipped
while still visible. Each edge has a broad 24%-wide quintic S-curve fade. The curved falloff
eases smoothly into both the background and the full-strength core. Independent slow
waves warp the edges by up to 1.2% of screen width. There is no bottom vignette.
The mask reduces both glyph density (and therefore the A/B color mix) and
opacity, so sparse B-colored edges dissolve into the background seamlessly.

```go
p := plasma.New(width, height) // terminal-cell dimensions

frame := p.Render()
view := frame.ANSI(
    plasma.RGB{R: 74, G: 222, B: 128}, // foreground
    plasma.RGB{R: 19, G: 21, B: 31},   // detected background
)

// Advance only after presenting the frame, as the browser version does.
p.Step()
```

For the browser's pixel-to-cell sizing, use `NewFromPixels(width, height)`.
`Frame.Text()` returns the character grid without ANSI escapes; `Frame.Cells`
retains the exact unrounded alpha plus its JavaScript-style three-decimal form.

The source animation increments time once per `requestAnimationFrame`, so it is
frame-rate-dependent. A host loop should render, call `Step`, and schedule its
next repaint. At 60 Hz this runs 30% faster than the source animation.

The underlying waves are ported from the published JavaScript; the horizontal
mask is applied before selecting glyphs and final alpha.
The final raster cannot be byte-identical across browser and terminal renderers:
font rasterization, canvas compositing, and the terminal's lack of alpha are
different. `Frame.Cells` is the renderer-neutral representation for exact use.

Run the included full-screen demo with:

```sh
go run ./cmd/plasma
```

To play a mask saved with `paint`, pass its filename:

```sh
go run ./cmd/plasma mask.json
```

This loads the shape, edge extensions, feather, distortion, rotation, speed, and
colors. Explicit flags override saved values (put flags before the filename):

```sh
go run ./cmd/plasma -rotation 45 -a '#4267d6' mask.json
```

Playback leaves the file unchanged. Without a filename, the original demo runs.

Press Ctrl-C, Escape, or `q` to exit. The command uses Bubble Tea v2's
`RequestBackgroundColor`; Bubble Tea reads the OSC 11 response through its
normal asynchronous input loop and delivers the exact color in a
`BackgroundColorMsg`. That RGB value is used directly as the opacity blend
color. Bubble Tea also owns raw mode, resizing, screen diffing, the alternate
screen, and terminal cleanup. The executable uses a cached 16-shade blend
palette; this cuts the ANSI view size substantially while leaving the plasma
glyph calculation unchanged.

Choose the two plasma colors with `-a` and `-b`:

```sh
go run ./cmd/plasma -a '#4267d6' -b '#00b3ff'
```

For terminal rendering, the A/B color mix and the glyph's visibility are two
independent axes. The mix ratio `m` comes from the glyph's position in the
density ramp (linear across the visible glyphs: the sparsest is pure B, the
densest is pure A), giving `m*A + (1-m)*B`. That color is then composited over
the detected background with strength `s`, the canvas opacity normalized from
its native `0...0.55` range to `0...1`. Consequently, a glyph's hue depends
only on how dense it is, while `s` only controls how strongly that hue shows
against the background — a faint, sparse glyph reads as B, not as a washed-out
A. The defaults are A `#4267d6` and B `#00b3ff` when no flags are supplied.

## Fast full-screen rendering

For a terminal you own, prefer `ScreenRenderer` over `Frame.ANSI`:

```go
screen := plasma.NewScreenRenderer(width, height, 24)
cells := make([]plasma.Cell, 0, width*height)
buffer := make([]byte, 0, width*height*2)

frame := p.RenderFastInto(cells)
cells = frame.Cells
buffer = screen.AppendFrame(buffer[:0], frame, foreground, background)
_, _ = output.Write(buffer)
```

This path precomputes static plasma geometry, reuses cell and output buffers,
quantizes only the terminal's simulated alpha colors, sends changed runs rather
than whole frames, caches color escape sequences, and wraps changes in terminal
synchronized-update markers. `Resize` forces the next update to repaint.

`Frame.ANSI` remains available when a self-contained view is required. It now
avoids maps and redundant adjacent color sequences, but inherently writes the
entire frame.

## Custom masks

Masks use coordinates relative to the component's current region: `(0, 0)` is
its top-left and `(1, 1)` its bottom-right. They automatically scale on `Resize`;
coordinates outside that range allow edges to extend beyond the viewport.

```go
// Start from the existing settings; change only the controls you need.
mask := plasma.DefaultMask()
mask.Feather = 0.12   // inward soft edge width; 0 gives a hard edge
mask.Distortion = 0.5 // half the original edge motion; 0 disables motion
mask.Path = new(plasma.Path).
    MoveTo(0.15, 0.5).
    CubicTo(0.15, 0.05, 0.85, 0.05, 0.85, 0.5).
    QuadTo(0.5, 1.1, 0.15, 0.5)
if err := p.SetMask(mask); err != nil {
    panic(err)
}
```

`Path` supports straight lines (`LineTo`), quadratic and cubic Bézier curves,
and multiple subpaths (`MoveTo`). Each subpath closes automatically. The even-odd
fill rule supports holes and disconnected regions. Curves are approximated to
0.0001 normalized units, with at most 16 subdivision levels. `SetMask` copies
the path; edits to the builder take effect only after another `SetMask` call.

For custom paths, feather width is measured as Euclidean distance in normalized
coordinates, so its pixel width depends on the region's aspect ratio. Distortion
moves the boundary inward and outward by up to `0.012 * Distortion` normalized
units using slow spatial waves. Feather and distortion must be finite and
nonnegative. An empty path hides the entire region.

A nil path keeps the original full-height band and its independent edge waves.
`p.SetMask(plasma.DefaultMask())` restores the original shape, 0.24 feather, and
1.0 distortion. Constructors keep these defaults, so existing callers retain
exactly the same appearance.

## Rotation

Rotate the plasma wave field before ASCII rasterization:

```go
if err := p.SetRotation(30); err != nil {
    panic(err)
}
```

Angles are degrees clockwise about the region's center; negative values rotate
counterclockwise. `Rotation()` returns the angle wrapped into `[0, 360)`.
Rotation changes the wave pattern and its movement inside the fixed mask,
using the component's `ColumnWidth` / `FontSize` proportions. The mask, its soft
edge, and its animated distortion stay in screen coordinates. Glyphs remain
upright because they are selected only after the transformed field is sampled.
The mask continues to bound the effect at every angle.
Zero is the default and preserves the original output exactly. Resizing keeps
the angle, and changing it preserves animation time.

The demo also accepts an angle:

```sh
go run ./cmd/plasma -rotation 30
```

## Paint a mask

Open a full-screen terminal editor with exactly one file argument:

```sh
go run ./cmd/paint mask.json
```

Or install the command, then run `paint mask.json`:

```sh
go install ./cmd/paint
```

An existing file is reopened; a new filename starts with an empty canvas and the
original feather, distortion, and plasma colors. **Tab** cycles through Canvas →
Configuration → Preview; **Shift-Tab** goes backward. All edits persist between
screens and terminal resizes. The terminal must be at least 48 columns by 16 rows.

- **Canvas:** click and drag to paint white on black. **E** toggles paint/erase,
  press **minus (`-`)** for a smaller brush or **plus (`+`)** for a larger brush,
  and **C** clears the canvas. The mouse wheel and bracket keys also adjust size.
  Press **B** to switch between regular paint and the **edge brush**. Drag along
  any screen border to mark it **bright red**: wherever white paint touches a
  marked interval, the shape continues infinitely outward instead of fading at
  the screen edge. The edge brush only marks the border; it does not add white
  paint. Markings over empty border areas take effect when paint reaches them.
  **E** erases both white paint and red markings with the same stroke; **C**
  clears both. Edge markings persist across screens, resizes, and save/reopen.
  The painted *filled area* becomes a simplified, closed vector path on release;
  overlapping strokes merge, and erased holes and separate islands are preserved.
- **Configuration:** **↑ / ↓** selects a variable; **← / →** adjusts numeric
  values. **Enter** opens a text field for exact values or hex colors, and applies
  the edit when pressed again. Typing also begins editing. **Esc** cancels the
  current edit. Fields support cursor movement, Home/End, Ctrl-A, and paste.
  Configure feather width, distortion intensity, animation speed, colors A/B,
  and rotation in degrees (5° arrow increments or any exact typed angle).
- **Preview:** the animated plasma fills the entire terminal with no controls
  overlaid. **Tab** or **Esc** returns to the canvas. The editor and preview use
  the terminal's default background. OSC 11 supplies the exact color for blending;
  terminals without that response use a dark blend fallback without changing the
  terminal background.

**Ctrl-S** saves from any screen. **Ctrl-X** (or Ctrl-C) exits; unsaved changes
prompt for save, discard, or cancel. Failed saves keep the editor open and retain
all edits. The footer's `*` marks unsaved changes.

Files are versioned JSON containing normalized `contours` (arrays of `{ "x": …,
"y": … }` points), `feather`, `distortion`, `speed`, `rotation`, `color_a`, `color_b`, and
`brush`, plus optional `edges` containing `top`, `right`, `bottom`, and `left`
arrays of `{ "start": …, "end": … }` normalized intervals. Top/bottom intervals
run left to right; left/right intervals run top to bottom. The renderer opens
matching boundary segments into infinite strips; adjacent markings meeting at
a corner also extend through that corner. Other boundaries keep their normal
feathering. Pass these intervals to `Mask.Extensions` (`plasma.EdgeExtensions`)
when loading a mask in Go. Older files without `rotation` load at 0°. The canvas keeps the original
geometry; the preview rotates the plasma movement inside that fixed shape. Contours use the same even-odd fill rule as `plasma.Path`, so they can be
rebuilt with `MoveTo` / `LineTo` and passed to `SetMask`. Saving uses an atomic
file replacement. The working paint surface is 256 × 256; its contours are
simplified within 0.4 working pixels and stored as vectors, independent of the
terminal's dimensions. Custom path distances are cached when the shape or grid
changes, keeping preview animation cost independent of vector complexity.
