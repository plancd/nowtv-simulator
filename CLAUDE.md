# CLAUDE.md — NOW TV Simulator

## Project overview

A Go program that simulates the HTTP control interface of a Roku-powered NOW TV /
Sky Stream set-top box.  It implements the Roku External Control Protocol (ECP)
so that the **Now Remote** iOS app (`../now-tv`) and similar tools can be tested
without needing physical hardware.

## Tech stack

| Layer        | Choice                                      |
|--------------|---------------------------------------------|
| Language     | Go 1.22+ (stdlib only — no external deps)   |
| HTTP         | `net/http` standard library                 |
| Discovery    | `net.ListenMulticastUDP` — SSDP UDP multicast on 239.255.255.250:1900 |
| Icon images  | `image/png` — procedurally generated        |
| XML          | Hand-built strings (no encoding/xml quirks) |

## Repository layout

```
main.go     Entry point: flag parsing, HTTP start, SSDP start, event loop + terminal UI
state.go    Mutable device state (Power, ActiveApp, event ring); all methods thread-safe
server.go   All ECP HTTP handlers + XML/PNG response builders
ssdp.go     SSDP M-SEARCH responder for auto-discovery
go.mod      Module declaration
```

## Development commands

```bash
go build ./...         # compile
go run .               # run with defaults (port 8060)
go run . --help        # list all flags
go test ./...          # run tests (if any)
```

## CLI flags

| Flag           | Default                              | Description                        |
|----------------|--------------------------------------|------------------------------------|
| `--port`       | `8060`                               | HTTP listen port                   |
| `--name`       | `Living Room NOW TV`                 | User-visible device name           |
| `--model`      | `NOW TV Box`                         | Model name (returned in device-info) |
| `--model-number` | `NOWTVBOX4K`                       | Model number                       |
| `--serial`     | `NTV20240001`                        | Serial number                      |
| `--udn`        | `015e5108-...`                       | UUID for SSDP USN                  |
| `--sw-version` | `9.2.0`                              | Reported software version          |
| `--lang`       | `en`                                 | Language code                      |
| `--country`    | `GB`                                 | Country code                       |

## ECP endpoints implemented

| Method | Path                    | Description                                  |
|--------|-------------------------|----------------------------------------------|
| POST   | `/keypress/{key}`       | Send a key press (updates power + home state) |
| POST   | `/keydown/{key}`        | Key held down (logged, no state change)       |
| POST   | `/keyup/{key}`          | Key released (logged, no state change)        |
| POST   | `/launch/{appId}`       | Switch active app                             |
| GET    | `/query/apps`           | XML list of installed channels                |
| GET    | `/query/active-app`     | XML of current foreground app                 |
| GET    | `/query/device-info`    | XML device metadata                           |
| GET    | `/query/icon/{appId}`   | PNG placeholder icon (290×218)                |
| GET    | `/`                     | Plain-text health check                       |

## State machine

- **Power** toggles between `PowerOn` and `Standby` via `Power`, `PowerOn`,
  `PowerOff` key presses.
- **ActiveApp** changes on `POST /launch/{id}`.  The `Home` key presses returns
  the device to the home screen (ActiveApp.ID = "").
- `/query/active-app` reports `<app>Roku</app>` (home) when in standby.

## Simulated channels

UK-focused NOW TV / Sky Stream app list — see `defaultApps` in `state.go`:

| ID            | Name          | Colour          |
|---------------|---------------|-----------------|
| 837           | NOW           | Dark blue        |
| 12            | Netflix       | Red              |
| 13            | Prime Video   | Cyan             |
| 2285          | Disney+       | Blue             |
| 195316        | YouTube       | Red              |
| 3423          | BBC iPlayer   | White            |
| 13871         | ITVX          | Blue             |
| 41468         | My5           | Blue             |
| 34376         | Spotify       | Green            |
| 12943         | Apple TV      | Dark             |
| 2594          | Plex          | Yellow           |
| tvinput.hdmi1 | HDMI 1        | (hash colour)    |
| tvinput.hdmi2 | HDMI 2        | (hash colour)    |

## Testing with the Now Remote app

1. Run the simulator on a Mac on the same WiFi as the iPhone:
   ```bash
   go run .
   ```
2. In Now Remote → Devices → Scan Network.
   The simulator responds to the SSDP M-SEARCH and appears in the list.
3. Alternatively: Devices → Add Manually → enter the Mac's IP, port 8060.

## Testing from the command line

```bash
# Keypress
curl -d '' http://localhost:8060/keypress/Home
curl -d '' http://localhost:8060/keypress/PowerOff

# Launch Netflix
curl -d '' http://localhost:8060/launch/12

# Query state
curl http://localhost:8060/query/active-app
curl http://localhost:8060/query/device-info

# Get channel icon
curl -o netflix.png http://localhost:8060/query/icon/12
```

## Design notes

- **No external dependencies** — stdlib only so there's no `go.sum` to manage.
- **SSDP failure is non-fatal** — if port 1900 is already bound, the simulator
  still works; discovery is just disabled.  Manual IP entry always works.
- **Icon generation** — brand colours are hard-coded for known IDs; unknown IDs
  get a deterministic colour derived from a hash of the ID string.
- **XML is hand-built** — avoids the encoding/xml package escaping attribute
  names with `_` → `-` in Go struct tags, which would break Roku parsers.
