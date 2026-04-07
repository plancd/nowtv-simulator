# NOW TV Simulator

[![Docker Hub](https://img.shields.io/docker/v/rikwatson/nowtv-simulator?label=Docker%20Hub&logo=docker)](https://hub.docker.com/r/rikwatson/nowtv-simulator)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://go.dev)

A lightweight Go program that simulates the HTTP control interface of a
Roku-powered NOW TV / Sky Stream set-top box.  It lets you develop and test
[Now Remote](https://github.com/rikwatson/now-tv) — or any other
Roku ECP client — without needing physical hardware.

## How it works

The [Roku External Control Protocol (ECP)](https://developer.roku.com/docs/developer-program/debugging/external-control-api.md)
is a plain HTTP API on port 8060.  This simulator:

- **Serves all ECP endpoints** — key presses, launches, device-info, apps list, active-app, channel icons
- **Responds to SSDP discovery** — appears automatically in the Now Remote app's "Scan Network" flow
- **Maintains live state** — power on/standby, currently active app
- **Logs every interaction** to the terminal with colour-coded output

## Quick start

```bash
# Requires Go 1.22+
go run .
```

The simulator starts on port 8060, announces itself on the local network via
SSDP, and prints a live event log:

```
──────────────────────────────────────────────────────────
  NOW TV Simulator
  Device   Living Room NOW TV  (NOW TV Box)
  Serial   NTV20240001
  Address  http://192.168.1.42:8060
  Port     8060   (ECP / Roku External Control Protocol)
  SSDP     listening on 239.255.255.250:1900 for auto-discovery
──────────────────────────────────────────────────────────
  Power    PowerOn
  App      Home (Roku)
──────────────────────────────────────────────────────────
...
Events:
  12:34:01  KEYPRESS   Home            192.168.1.10
  12:34:02  LAUNCH     Netflix (id=12) 192.168.1.10
  12:34:05  QUERY      active-app      192.168.1.10
```

## Options

```
go run . --help

  --port           HTTP port (default 8060)
  --name           Device name shown in app (default "Living Room NOW TV")
  --model          Model name (default "NOW TV Box")
  --model-number   Model number (default "NOWTVBOX4K")
  --serial         Serial number (default "NTV20240001")
  --sw-version     Software version (default "9.2.0")
  --lang           Language code (default "en")
  --country        Country code (default "GB")
  --advertise-ip   IP to advertise in SSDP and banner.
                   Set this to your host machine's LAN IP when running
                   in Docker so Now Remote can actually reach the simulator.
```

Run multiple instances on different ports to simulate multiple devices:

```bash
go run . --port 8060 --name "Living Room"
go run . --port 8061 --name "Bedroom" --serial "NTV20240002"
```

## ECP endpoints

| Method | Path                    | Description                                  |
|--------|-------------------------|----------------------------------------------|
| POST   | `/keypress/{key}`       | Key press (updates power/home state)          |
| POST   | `/keydown/{key}`        | Key held down                                 |
| POST   | `/keyup/{key}`          | Key released                                  |
| POST   | `/launch/{appId}`       | Switch active app                             |
| GET    | `/query/apps`           | XML list of installed channels                |
| GET    | `/query/active-app`     | XML of current foreground app                 |
| GET    | `/query/device-info`    | XML device metadata                           |
| GET    | `/query/icon/{appId}`   | PNG placeholder icon (290×218)                |

### Supported keys

`Home` `Back` `Select` `Up` `Down` `Left` `Right` `Play` `Rev` `Fwd`
`InstantReplay` `Info` `Search` `VolumeUp` `VolumeDown` `VolumeMute`
`Power` `PowerOn` `PowerOff` `Backspace` `Enter`

## Testing with curl

```bash
# Key presses
curl -d '' http://localhost:8060/keypress/Home
curl -d '' http://localhost:8060/keypress/VolumeUp
curl -d '' http://localhost:8060/keypress/PowerOff

# Launch Netflix (id=12)
curl -d '' http://localhost:8060/launch/12

# Query state
curl http://localhost:8060/query/active-app
curl http://localhost:8060/query/device-info
curl http://localhost:8060/query/apps

# Download a channel icon
curl -o icon.png http://localhost:8060/query/icon/12
```

## Testing with Now Remote

1. Run the simulator on a Mac connected to the same WiFi as your iPhone.
2. In Now Remote → **Devices** → **Scan Network**.
3. The simulator responds to the SSDP broadcast and appears in the list automatically.
4. Alternatively: **Add Manually** → enter your Mac's IP address.

## Simulated channel list

UK-focused NOW TV / Sky Stream library:

| Channel     | App ID        |
|-------------|---------------|
| NOW         | 837           |
| Netflix     | 12            |
| Prime Video | 13            |
| Disney+     | 2285          |
| YouTube     | 195316        |
| BBC iPlayer | 3423          |
| ITVX        | 13871         |
| My5         | 41468         |
| Spotify     | 34376         |
| Apple TV    | 12943         |
| Plex        | 2594          |
| HDMI 1      | tvinput.hdmi1 |
| HDMI 2      | tvinput.hdmi2 |

## Docker

The easiest way to run the simulator — no Go toolchain needed.  The image is
published automatically to Docker Hub on every push to `main`.

### Pull from Docker Hub

```bash
docker run -p 8060:8060 rikwatson/nowtv-simulator
```

With a custom device name:

```bash
docker run -p 8060:8060 rikwatson/nowtv-simulator \
  --name "Bedroom NOW TV" --serial "NTV20240002"
```

Available tags: `latest` (main branch), semver tags (`1.0.0`, `1.0`) once
version tags are pushed to git.

#### Running in Docker — making the simulator discoverable

Inside a Docker container the simulator detects the container's internal IP
(e.g. `172.17.0.2`), which is unreachable from your iPhone.  Pass
`--advertise-ip` set to your **Mac's LAN IP** so SSDP responses and the banner
show the correct address:

```bash
# Find your Mac's LAN IP first
ipconfig getifaddr en0

# Then run with that IP
docker run -p 8060:8060 rikwatson/nowtv-simulator \
  --name "Living Room NOW TV" \
  --advertise-ip 192.168.1.42
```

Now Remote → **Add Manually** → enter the same IP (`192.168.1.42`), port 8060.
SSDP auto-discovery still won't work on macOS Docker Desktop (UDP multicast
limitation), but manual IP entry works perfectly.

### docker-compose

```bash
docker compose up
```

Edit `docker-compose.yml` to run multiple simulators on different ports.

### SSDP discovery in Docker

SSDP uses UDP multicast, which doesn't cross the Docker network bridge by
default.

| Platform | How to enable discovery |
|---|---|
| **Linux** | Add `network_mode: host` to the compose service |
| **macOS / Windows Docker Desktop** | Not supported — use **Add Manually** in Now Remote instead |

Manual IP entry always works regardless of networking mode.

### Building the image locally

```bash
docker build -t nowtv-simulator .
docker run -p 8060:8060 nowtv-simulator
```

### Multi-arch build (amd64 + arm64)

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t rikwatson/nowtv-simulator:latest --push .
```

The GitHub Actions workflow does this automatically on every push to `main`.

## Building a binary

```bash
go build -o nowtv-simulator .
./nowtv-simulator --name "Test Device"
```

## Requirements

- Go 1.22 or later  **or**  Docker
- No external Go dependencies — stdlib only

## TODO

Things that would make this more useful or production-grade:

### Testing
- [ ] Unit tests for `state.go` — key-press state machine, event ring buffer
- [ ] HTTP integration tests for every ECP endpoint (table-driven)
- [ ] XML output tests — validate well-formedness and field values
- [ ] Concurrent-access tests to exercise the mutex paths

### Features
- [ ] `--apps` flag to load a custom channel list from a JSON file (makes it easy to mirror a real device)
- [ ] `POST /input` endpoint — text input for search fields
- [ ] `GET /query/media-player` — stub for media player state queries
- [ ] Web status page at `/ui` — shows current state in a browser without a terminal
- [ ] Webhook / callback URL — POST event JSON to a configured URL so CI pipelines can assert on received commands

### Reliability
- [ ] Graceful shutdown of the SSDP goroutine (currently runs until process exit)
- [ ] `--version` flag embedding build time and git SHA via `ldflags`
- [ ] Rate limiting on ECP endpoints

### Docker / deployment
- [ ] Docker Hub description auto-sync from README (using `peter-evans/dockerhub-description`)
- [ ] Version tags on Docker Hub tied to git semver tags (`v1.0.0` → `1.0.0`, `1.0`, `latest`)
- [ ] Helm chart / Kubernetes manifest for running in a test cluster

## Related

- [Now Remote (iOS app)](https://github.com/rikwatson/now-tv)
- [Roku ECP documentation](https://developer.roku.com/docs/developer-program/debugging/external-control-api.md)
- [Docker Hub — rikwatson/nowtv-simulator](https://hub.docker.com/r/rikwatson/nowtv-simulator)
