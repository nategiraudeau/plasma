# plasma

ascii plasma for the terminal. includes a mask editor and a reusable go component.

## run

requires go 1.25 or newer.

```sh
go run ./cmd/plasma
```

run a saved mask:

```sh
go run ./cmd/plasma mask.json
```

saved files include the shape, edge extensions, colors, feather, distortion, speed, and rotation. flags override saved settings:

```sh
go run ./cmd/plasma -a '#4267d6' -b '#00b3ff' -rotation 30 mask.json
```

rotation changes the plasma's movement inside the mask. the mask stays fixed.

press `q`, `esc`, or `ctrl-c` to exit.

## paint

```sh
go run ./cmd/paint mask.json
```

opens an existing file or starts a new shape. needs a terminal at least 48 columns by 16 rows.

- `tab`: switch between canvas, configuration, and preview
- click and drag: paint the shape in white
- `-` / `+`: smaller / larger brush
- `b`: switch between paint and the edge brush
- `e`: erase both paint and edge markings
- `c`: clear everything
- `ctrl-s`: save
- `ctrl-x`: exit

the edge brush marks screen borders in red. where paint touches a marked border, the shape extends infinitely beyond it.

in configuration, use `↑` / `↓` to select a setting, `←` / `→` to adjust it, and `enter` to type an exact value. `esc` cancels the edit.

shapes are saved as vector paths in json. edits persist between screens, and unsaved changes prompt before exit.

## use in go

```go
p := plasma.New(80, 24)
frame := p.Render()
fmt.Print(frame.Text())
p.Step()
```

`SetMask` accepts vector paths, feather width, distortion, and edge extensions. `SetRotation` changes the wave direction in clockwise degrees. `Resize` preserves the animation state.

## test

```sh
go test ./...
```
