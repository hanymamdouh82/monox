# MONOX

A lightweight, zero-fork, high-performance Terminal User Interface (TUI) dashboard tailored for homelab telemetry. Built in native Go using `gocui`, it minimizes CPU overhead by parsing host metrics directly from `/proc` filesystems.

## Features

- **Grid Layout:** 2-column x 4-row dashboard monitoring Temps, Memory, Docker statuses, Syncthing synchronization progress, SMART drive diagnostics, Disk space utilization, and System loads.
- **Low Footprint:** Replaced traditional polling metrics libraries with optimized state-tracking `/proc` listeners, dropping steady-state CPU utilization below 0.5%.
- **Interactive Navigation:** Supports interactive mouse-wheel viewport adjustments and standard keyboard cycling (`Tab` / `Shift+Tab`) with color-coded focused frame boundaries.

## Installation & Static Compilation

To build a fully self-contained, statically linked binary independent of the host runtime or dynamic C libraries (ideal for seamless migration to remote Ubuntu server environments):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -extldflags '-static'" -o monox .
```

## Configuration

The dashboard uses a structured YAML template for target path inputs. Duplicate the example file and update it with your environment parameters:

```bash
cp config.yaml.example config.yaml

```

### `config.yaml` Schema

```yaml
name: <your_desired_screen_name>
syncthing:
  url: "[http://127.0.0.1:8384](http://127.0.0.1:8384)"
  api_key: "your-syncthing-api-key"

smart:
  drives:
    - sda
    - sdb

disk:
  mounts:
    - /
    - /mnt/data
```

## Usage

Run the executable. By default, it looks for a local `config.yaml` file in the execution scope:

```bash
./monox

```

To provide an explicit configuration path configuration, pass the `-config` flag:

```bash
./monox -config /etc/monox/production.yaml

```

### Navigation Map

- **`Tab`** : Cycle focus forward to the next grid tile.
- **`Alt + Tab`** : Cycle focus backward to the previous grid tile.
- **`Left Mouse Click`** : Instantly lock window focus to the target panel layout bounding area.
- **`Arrow Up / Down`** or **`Mouse Wheel`** : Scroll vertically through overflowing text in the active pane.
- **`q`** or **`Ctrl + C`** : Safely shut down the layout engine loop and restore shell state.

## License

Distributed under the MIT License. See `LICENSE` for details.
