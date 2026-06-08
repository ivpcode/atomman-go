# AtomMan Go

A Go daemon for the secondary touch display of the **AtomMan X7 Ti** (and compatible models) on Linux.

The display communicates over USB serial using a proprietary tile-based protocol. This daemon reads system metrics and sends them to the display in real time.

---

## Features

- **CPU** — model, temperature, usage, frequency
- **GPU** — auto-detection for NVIDIA / AMD / Intel Arc (i915 / xe drivers)
- **RAM** — manufacturer, used / available / total, percentage
- **Disk** — NVMe model, temperature, used / total space
- **Date/Time** — date, time, weekday, optional weather
- **Network** — RX/TX speed, fan RPM
- **Volume** — system audio level (PulseAudio/PipeWire)
- **Battery** — percentage (useful on laptops; disabled by default on desktops)
- **Terminal dashboard** — live ANSI monitor via `--dashboard`

---

## Requirements

| Dependency | Purpose |
|---|---|
| Go ≥ 1.21 | build |
| `dialout` group | access to `/dev/ttyACM0` |
| systemd | service management |
| `pactl` (optional) | volume tile |
| `dmidecode` (optional) | RAM label |
| `intel_gpu_top` (optional) | accurate Intel GPU utilization |

---

## Installation

```bash
git clone https://github.com/ivpcode/atomman-go.git
cd atomman-go
bash scripts/install.sh
```

`install.sh` builds the binary, copies it to `/opt/atomman-go/`, adds your user to the `dialout` group, and creates and starts the `atomman-go` systemd service.

> **Note:** You may need to log out and back in for the `dialout` group change to take effect.

---

## Service Management

### Status

```bash
sudo systemctl status atomman-go
```

### Start / Stop / Restart

```bash
sudo systemctl start atomman-go
sudo systemctl stop atomman-go
sudo systemctl restart atomman-go
```

### Enable / Disable autostart at boot

```bash
sudo systemctl enable atomman-go    # start automatically on boot (set by install.sh)
sudo systemctl disable atomman-go   # do not start at boot
```

### Live logs

```bash
journalctl -u atomman-go -f
```

### View last 50 log lines

```bash
journalctl -u atomman-go -n 50 --no-pager
```

---

## Update after code changes

```bash
bash scripts/update.sh
```

Rebuilds the binary and restarts the service without touching the installed config.

---

## Uninstall

```bash
bash scripts/uninstall.sh
```

Stops and disables the service, removes the service file and `/opt/atomman-go/`.

---

## Configuration

Edit `/opt/atomman-go/config.yaml` (installed copy) or `config.yaml` in the source directory before running `install.sh`:

```yaml
serial:
  port: /dev/ttyACM0      # display USB port
  start_delay: 5.0        # seconds to wait before opening port

tiles:                    # tiles to show, in order
  - cpu
  - gpu
  - memory
  - disk
  - datetime
  - network
  - volume
  # - battery             # enable only on laptops

custom_labels:            # override auto-detected names
  cpu:    ""
  gpu:    ""
  memory: ""
  disk:   ""

weather:
  api_key: ""             # OpenWeatherMap API key (optional)
  location: "Rome,IT"

fan:
  prefer: auto            # auto | hwmon | nvidia
  max_rpm: 2000

network:
  interface: ""           # empty = auto-detect
```

After editing the installed config:
```bash
sudo systemctl restart atomman-go
```

---

## Manual Usage

```bash
# Start with terminal dashboard and reduced start delay
./atomman --dashboard --start-delay 3

# Available flags
  -config string       path to YAML config (default "config.yaml")
  -port string         serial port (overrides config)
  -start-delay float   startup delay in seconds
  -dashboard           show live terminal monitor
  -no-color            disable ANSI colors
  -attempts int        display unlock attempts
  -window float        unlock window in seconds
  -fan-max-rpm int     max fan RPM for NVIDIA
  -fan-prefer string   fan source: auto|hwmon|nvidia
```

---

## Project Structure

```
atomman-go/
├── main.go        # entry point, main loop, unlock sequence
├── protocol.go    # serial protocol (ENQ/reply, framing)
├── metrics.go     # metric reading from /proc, /sys, hwmon
├── tiles.go       # payload builders for each tile
├── dashboard.go   # live ANSI terminal monitor
├── weather.go     # OpenWeatherMap integration (optional)
├── config.go      # Config struct + YAML loader
├── config.yaml    # user configuration
└── scripts/
    ├── install.sh   # build + install systemd service
    ├── update.sh    # rebuild and redeploy binary
    └── uninstall.sh # stop service and remove all files
```

---

## Serial Protocol

The display identifies as `0416:50a1 Winbond Electronics Corp. USB Virtual COM` on `/dev/ttyACM0`.

```
Display → Host  (ENQ):     AA 05 <seq> CC 33 C3 3C
Host    → Display (reply): AA <tileID> 00 <seq> <ASCII payload> CC 33 C3 3C
```

| Tile | ID | Example payload |
|---|---|---|
| CPU | `0x53` | `{CPU:Intel Core Ultra 9;Tempr:52;Useage:12;Freq:2400000;Tempr1:52;}` |
| GPU | `0x36` | `{GPU:Intel Arc Graphics;Tempr:;Useage:8}` |
| RAM | `0x49` | `{Memory:Micron;Used:14.2;Available:17.8;Total:32.0;Useage:44}` |
| Disk | `0x4F` | `{DiskName:Samsung 990 Pro;Tempr:38;UsageSpace:512;AllSpace:2048;Usage:25}` |
| Date | `0x6B` | `{Date:2024/06/08;Time:21:30:00;Week:6;Weather:1;TemprLo:18,TemprHi:28,...}` |
| Network | `0x27` | `{SPEED:1200;NETWORK:1.2 M/s,0.3 M/s}` |
| Volume | `0x10` | `{VOLUME:75}` |
| Battery | `0x1A` | `{Battery:85}` |

Protocol reverse-engineered from [RamSet/AtomMan](https://github.com/RamSet/AtomMan).

---

## License

MIT
