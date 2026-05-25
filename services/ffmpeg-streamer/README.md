# ffmpeg-streamer

Reads live JPEG frames from the shared IPC socket and pipes them to `ffmpeg` for RTMP streaming to Twitch, YouTube, or any custom endpoint.

## Overview

`ffmpeg-streamer` connects to the same Unix IPC socket as `camera-web-api` and operates in one of two modes:

- **Single-camera** — streams the latest JPEG for one camera ID to an RTMP endpoint.
- **Merge** — composites all active cameras into a single grid canvas and streams that.

In both modes the IPC reader runs independently from the `ffmpeg` process. If `ffmpeg` exits it is automatically restarted after a configurable delay without interrupting the IPC connection.

```text
                    ┌─ ffmpeg-streamer ──────────────────────────────────┐
Unix socket ──── ipcReader ─────┬─ single mode: latest(cameraID)        │
(IPC frames)                    │                                         ├──► ffmpeg ──► RTMP
                                └─ merge mode: allLatest() → grid canvas │
                                                                          └────────────────────
```

## Modes

### Single-camera (default)

Streams the camera specified by `CAMERA_ID` / `--camera-id`. Frames are fed to `ffmpeg` at exactly `STREAM_FRAME_RATE` fps. If the camera is silent the last received frame is held and repeated; if no frame has ever arrived nothing is sent.

### Merge (`--merge` / `MERGE_ALL=true`)

Discovers all active cameras from the IPC stream at runtime and places them in a square grid. Each cell is `CELL_WIDTH × CELL_HEIGHT` pixels; frames are letterboxed to fit. The canvas is recomposed only when new frames arrive (version-counter check), so CPU usage scales with actual frame activity rather than the output frame rate.

```text
  2 cameras → 2×1 grid      4 cameras → 2×2 grid      7 cameras → 3×3 grid
  ┌─────┬─────┐             ┌─────┬─────┐             ┌─────┬─────┬─────┐
  │  0  │  1  │             │  0  │  1  │             │  0  │  1  │  2  │
  └─────┴─────┘             ├─────┼─────┤             ├─────┼─────┼─────┤
                            │  2  │  3  │             │  3  │  4  │  5  │
                            └─────┴─────┘             ├─────┼─────┼─────┤
                                                      │  6  │     │     │
                                                      └─────┴─────┴─────┘
```

## Stream destination

Exactly one of the following must be set:

| Variable | Example | Description |
| --- | --- | --- |
| `STREAM_URL` | `rtmp://host/app/key` | Generic RTMP URL |
| `RTMP_URL` | `rtmp://host/app/key` | Alias for `STREAM_URL` |
| `TWITCH_STREAM_KEY` | `live_...` | Streams to `rtmp://live.twitch.tv/app/<key>` |
| `YOUTUBE_STREAM_KEY` | `xxxx-...` | Streams to `rtmp://a.rtmp.youtube.com/live2/<key>` |

## Configuration

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `IPC_SOCKET_PATH` | `/tmp/instachron/frames.sock` | Unix socket path (must match `tcp-camera-backend`) |
| `CAMERA_ID` | `0` | Camera to stream in single-camera mode |
| `STREAM_FRAME_RATE` | `10` | Output fps fed to `ffmpeg` |
| `FFMPEG_PATH` | `ffmpeg` | Path to the `ffmpeg` binary |
| `FFMPEG_RESTART_DELAY` | `5s` | Wait between `ffmpeg` restarts on failure |
| `MERGE_ALL` | `false` | Enable multi-camera grid canvas mode |
| `CELL_WIDTH` | `320` | Grid cell width in pixels (rounded up to even) |
| `CELL_HEIGHT` | `240` | Grid cell height in pixels (rounded up to even) |

### Flags

| Flag | Description |
| --- | --- |
| `--camera-id <n>` | Override `CAMERA_ID` |
| `--merge` | Enable merge mode (overrides `MERGE_ALL`) |
| `--cell-width <n>` | Override `CELL_WIDTH` |
| `--cell-height <n>` | Override `CELL_HEIGHT` |

## Building and running

```sh
go build -o ffmpeg-streamer ./cmd/ffmpeg-streamer
```

Load the root environment example before running locally:

```sh
set -a
source ../../.env.example
set +a
```

Stream a single camera to Twitch:

```sh
TWITCH_STREAM_KEY=live_xxxx \
CAMERA_ID=0 \
./ffmpeg-streamer
```

Merge all cameras and stream to a custom endpoint:

```sh
STREAM_URL=rtmp://192.168.1.10/live/key \
MERGE_ALL=true \
CELL_WIDTH=640 CELL_HEIGHT=480 \
./ffmpeg-streamer --merge
```

Override the camera at runtime:

```sh
TWITCH_STREAM_KEY=live_xxxx ./ffmpeg-streamer --camera-id 2
```

## Module layout

| File | Responsibility |
| --- | --- |
| `cmd/ffmpeg-streamer/main.go` | Process entry point |
| `internal/app/run.go` | Config loading, `ffmpeg` lifecycle loop, single-camera frame feed |
| `internal/app/ipc.go` | IPC Unix socket client with reconnect; stores latest JPEG per camera in a lock-protected map |
| `internal/app/merge.go` | Multi-camera canvas composition — grid layout, letterbox scaling, JPEG encode |
