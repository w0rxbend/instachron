# camera-web-api

HTTP server that consumes JPEG frames from `tcp-camera-backend` over a Unix IPC socket and serves live MJPEG streams, snapshots, and a browser-based viewer.

## Overview

`camera-web-api` connects to the shared IPC socket as a consumer. Incoming frames are optionally rotated server-side (configured per camera ID), then pushed to every active HTTP subscriber. Cameras are discovered automatically the moment their first frame arrives; they remain visible in the API even when offline so clients can display a clear "offline" state.

```text
                               ┌─ camera-web-api ──────────────────────┐
Unix socket ──── socketReader ─┤                                        ├── GET /cameras
(IPC frames)                   │  hubManager                            ├── GET /cameras/{id}/snapshot
                               │    dispatch → rotate → hub.push()     ├── GET /cameras/{id}/stream
                               │    liveness check (5 s)               └── GET /
                               └───────────────────────────────────────┘
```

## Camera discovery and liveness

- A camera becomes **known** the moment its first IPC frame arrives — no filesystem scanning.
- A camera is marked **offline** immediately when `tcp-camera-backend` sends a `CAMERA_OFFLINE` notification (i.e. the ESP32's TCP connection dropped).
- A fallback liveness goroutine marks any camera that has been silent for more than 5 seconds as offline, guarding against mid-stream IPC connection loss.
- When the IPC connection itself drops, all cameras are marked offline instantly and a reconnect loop retries every second.
- Cameras are **never removed** from the known list — offline cameras stay visible in `/cameras` and the web UI.

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/` | Browser viewer (served from `WEB_DIR/index.html`) |
| `GET` | `/cameras` | JSON array of all known cameras including offline ones |
| `GET` | `/cameras/{id}/snapshot` | Latest JPEG from memory; served even when camera is offline |
| `GET` | `/cameras/{id}/stream` | MJPEG `multipart/x-mixed-replace` stream |

### `/cameras` response

```json
[
  { "id": "0", "index": 0, "online": true,  "rotation": 180 },
  { "id": "1", "index": 1, "online": false, "rotation": 0   }
]
```

| Field | Description |
| --- | --- |
| `id` | Camera ID string (decimal camera ID from IPC protocol) |
| `index` | Stable zero-based position in the sorted camera list |
| `online` | `true` if frames are actively arriving |
| `rotation` | Configured clockwise rotation in degrees (0 / 90 / 180 / 270) |

## Rotation config

Per-camera server-side rotation is configured in a JSON file (default `./cameras.json`). Any multiple of 90° is accepted; values are normalised to `0 / 90 / 180 / 270`.

```json
{
  "0": 180,
  "1": -90,
  "3": 270
}
```

Rotation is applied once per incoming frame before broadcasting to subscribers — no per-subscriber overhead. The service must be restarted to pick up config changes.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | HTTP listen address |
| `IPC_SOCKET_PATH` | `/tmp/instachron/frames.sock` | Unix socket path (must match `tcp-camera-backend`) |
| `WEB_DIR` | `./web` | Directory containing `index.html` |
| `CAMERA_CONFIG` | `./cameras.json` | Per-camera rotation config (missing file = no rotation) |

## Building and running

```sh
go build -o camera-web-api .
./camera-web-api
```

```sh
HTTP_ADDR=:8080 \
IPC_SOCKET_PATH=/run/instachron/frames.sock \
CAMERA_CONFIG=/etc/instachron/cameras.json \
./camera-web-api
```

## Module layout

| File | Responsibility |
| --- | --- |
| `main.go` | Entry point — env config, signal handling, wires hub manager, socket reader, HTTP server |
| `socket_reader.go` | IPC Unix socket client with automatic reconnect; dispatches FRAME and OFFLINE messages |
| `hub.go` | Per-camera push hubs; `hubManager` owns liveness checks and camera registry |
| `handler.go` | HTTP routes, MJPEG frame writer, embedded web UI |
| `rotation.go` | Config file loading, angle normalisation, JPEG decode/rotate/encode pipeline |
