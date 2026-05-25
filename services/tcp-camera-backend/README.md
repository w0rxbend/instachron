# tcp-camera-backend

Receives JPEG frames from ESP32-CAM devices over a custom binary TCP protocol and publishes them to a Unix domain socket consumed by `camera-web-api` and `ffmpeg-streamer`.

## Overview

The server accepts raw TCP connections — one per camera, or multiple cameras per connection using the JPGD protocol variant. Each validated JPEG frame is forwarded immediately to every connected IPC consumer with sub-millisecond latency. When a camera's TCP connection drops, all consumers receive a `CAMERA_OFFLINE` notification.

```text
ESP32-CAM #0 ──┐
ESP32-CAM #1 ──┤  TCP :5000   ┌─ tcp-camera-backend ─┐   Unix socket   ┌── camera-web-api
ESP32-CAM #N ──┘              │  receive · validate   ├─────────────────┤── ffmpeg-streamer
                              │  publish to IPC       │                 └── any consumer
                              └───────────────────────┘
```

## Wire protocol (ESP32-CAM → backend)

Two frame formats are accepted on the same port, distinguished by the first 4 magic bytes. All fields are big-endian `uint32`.

### JPGS — legacy (single camera, always camera ID 0)

```text
 0       4       8       12      16
 +-------+-------+-------+-------+
 | magic | seq   | size  | ts_ms |   ← 16-byte header
 +-------+-------+-------+-------+
 |        JPEG payload            |   ← size bytes
 +--------------------------------+

magic = 0x4A504753  ("JPGS")
```

### JPGD — multi-camera

```text
 0       4       8       12      16      20
 +-------+-------+-------+-------+-------+
 | magic | seq   | size  | ts_ms | camID |   ← 20-byte header
 +-------+-------+-------+-------+-------+
 |          JPEG payload                  |   ← size bytes
 +----------------------------------------+

magic = 0x4A504744  ("JPGD")
```

## IPC protocol (backend → consumers)

Frames are forwarded over a Unix domain socket using a simple binary framing protocol. All multi-byte fields are big-endian.

```text
 0    1    2    3        6        10
 +----+----+----+--------+--------+---------- - -
 | AA | BB | tp | camID  | size   | payload
 +----+----+----+--------+--------+---------- - -

tp = 0x01  FRAME    — JPEG follows (size bytes)
tp = 0x02  OFFLINE  — camera disconnected (size = 0, no payload)
```

Multiple consumers connect to the socket simultaneously. Each gets its own buffered channel (64 messages); slow consumers drop frames rather than back-pressure the ingest path.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `TCP_ADDR` | `0.0.0.0:5000` | TCP listen address for camera connections |
| `IPC_SOCKET_PATH` | `/tmp/instachron/frames.sock` | Unix socket path for IPC consumers |
| `MAX_FRAME_BYTES` | `5242880` (5 MiB) | Maximum accepted JPEG payload size |
| `READ_TIMEOUT` | `30s` | Per-read deadline; silent cameras are disconnected and marked offline |

## Building and running

```sh
go build -o tcp-camera-backend ./cmd/tcp-camera-backend
./tcp-camera-backend
```

```sh
TCP_ADDR=0.0.0.0:5000 \
IPC_SOCKET_PATH=/run/instachron/frames.sock \
./tcp-camera-backend
```

## Module layout

| File | Responsibility |
| --- | --- |
| `cmd/tcp-camera-backend/main.go` | Process entry point |
| `internal/app/run.go` | Signal handling and process wiring |
| `internal/config/config.go` | Environment configuration loading |
| `internal/server/server.go` | TCP listener, per-connection goroutines, frame validation, offline notification |
| `internal/protocol/protocol.go` | Binary header parsing for JPGS/JPGD variants, JPEG SOI/EOI marker check |
| `internal/publisher/publisher.go` | Unix socket IPC server, per-consumer buffered fan-out |
