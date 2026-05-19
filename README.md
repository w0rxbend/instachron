# instachron

TCP frame server for ESP32-CAM JPEG frames.

## Workspace

This repository is a Go workspace with two project modules:

- `tcp-camera-backend`: raw TCP receiver for ESP32-CAM JPEG frames.
- `ffmpeg-streamer`: streaming project module.

From the repository root, workspace-aware Go commands can target either module:

```sh
go test ./tcp-camera-backend/...
go run ./tcp-camera-backend
go run ./ffmpeg-streamer
```

## Client-server protocol

Transport is a long-lived raw TCP connection. There is no HTTP, websocket, JSON, delimiter, or text framing.

The ESP32 client connects to the backend TCP listener, default `0.0.0.0:5000`, and sends repeated binary frames:

```text
bytes 0..3     magic: 0x4A504753, ASCII "JPGS", uint32 big-endian
bytes 4..7     sequence number, uint32 big-endian
bytes 8..11    JPEG payload length in bytes, uint32 big-endian
bytes 12..15   camera millis() timestamp, uint32 big-endian
bytes 16..N    JPEG payload, exactly payload length bytes
```

After one frame is received, the next frame starts immediately with another 16-byte header on the same TCP stream.

Backend receiver rules:

- Read exactly 16 bytes for the header.
- Decode all header fields as big-endian `uint32`.
- Reject the connection if magic is not `JPGS`.
- Reject payloads larger than `MAX_FRAME_BYTES`.
- Read exactly `payload length` bytes for the JPEG.
- Validate JPEG start/end markers before writing.
- Treat TCP disconnects as normal; the ESP32 client reconnects and continues streaming later frames.

The sequence number can be used to detect dropped frames. The timestamp is the ESP32 `millis()` value when the frame header was built; it is not wall-clock time.

Each accepted JPEG is written to `FRAME_OUTPUT_DIR`, and the latest image is atomically updated at `CURRENT_IMAGE_PATH`.

## Running

Run the server:

```sh
go run ./tcp-camera-backend
```

Useful environment variables:

```sh
TCP_ADDR=0.0.0.0:5000
FRAME_OUTPUT_DIR=./frames
CURRENT_IMAGE_PATH=./current-image.jpeg
MAX_FRAME_BYTES=5242880
READ_TIMEOUT=30s
```

## Streaming

Run the ffmpeg daemon from the repository root:

```sh
STREAM_URL=rtmp://example/live/key go run ./ffmpeg-streamer
```

The streamer watches `FRAME_OUTPUT_DIR`, reads the newest `.jpg` or `.jpeg` frame, feeds frames to `ffmpeg` over stdin as MJPEG, and publishes an RTMP stream. You can set one direct output URL or one platform stream key:

```sh
STREAM_URL=rtmp://example/live/key
RTMP_URL=rtmp://example/live/key
TWITCH_STREAM_KEY=live_xxx
YOUTUBE_STREAM_KEY=xxxx-xxxx-xxxx-xxxx
```

Useful streamer environment variables:

```sh
FRAME_OUTPUT_DIR=./frames
FFMPEG_PATH=ffmpeg
STREAM_FRAME_RATE=10
FRAME_POLL_INTERVAL=250ms
FFMPEG_RESTART_DELAY=5s
```
