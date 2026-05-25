# instachron

TCP frame server for ESP32-CAM JPEG frames.

## Workspace

This repository is a Go workspace with independently deployable service modules under `services/`:

- `services/tcp-camera-backend`: raw TCP receiver for ESP32-CAM JPEG frames and Unix socket publisher.
- `services/camera-web-api`: HTTP API and web UI backed by the frame IPC socket.
- `services/camera-web-restreamer-api`: HTTP restream proxy for camera streams.
- `services/camera-web-restream-enhancer-api`: restream proxy with image enhancement.
- `services/camera-web-restream-fsrcnn-api`: optional FSRCNN upscaling restream proxy.
- `services/ffmpeg-streamer`: ffmpeg RTMP streaming process.

From the repository root, workspace-aware Go commands can target any service command:

```sh
go test ./services/tcp-camera-backend/...
go run ./services/tcp-camera-backend/cmd/tcp-camera-backend
go run ./services/ffmpeg-streamer/cmd/ffmpeg-streamer
```

## Client-server protocol

Transport is a long-lived raw TCP connection. There is no HTTP, websocket, JSON, delimiter, or text framing.

The ESP32 client connects to the backend TCP listener, default `0.0.0.0:5000`, and sends repeated binary frames. The current multi-camera protocol is:

```text
bytes 0..3       magic: 0x4A504744, ASCII "JPGD", uint32 big-endian
bytes 4..7       sequence number, uint32 big-endian
bytes 8..11      JPEG payload length in bytes, uint32 big-endian
bytes 12..15     camera millis() timestamp, uint32 big-endian
bytes 16..19     camera id, uint32 big-endian
bytes 20..N      JPEG payload, exactly payload length bytes
```

For compatibility, the backend still accepts the legacy single-camera protocol as camera id `0`:

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
- Decode all fixed-width fields as big-endian `uint32`.
- If magic is `JPGD`, read the 4-byte camera id before the JPEG payload.
- Reject the connection if magic is not `JPGD` or legacy `JPGS`.
- Reject payloads larger than `MAX_FRAME_BYTES`.
- Read exactly `payload length` bytes for the JPEG.
- Validate JPEG start/end markers before writing.
- Treat TCP disconnects as normal; the ESP32 client reconnects and continues streaming later frames.

The sequence number can be used to detect dropped frames. The timestamp is the ESP32 `millis()` value when the frame header was built; it is not wall-clock time.

Each accepted JPEG is published to connected consumers over the Unix socket configured by `IPC_SOCKET_PATH`.

## Running

Create a local environment file:

```sh
cp .env.example .env
```

Run the server:

```sh
go run ./services/tcp-camera-backend/cmd/tcp-camera-backend
```

Useful environment variables:

```sh
TCP_ADDR=0.0.0.0:5000
IPC_SOCKET_PATH=/tmp/instachron/frames.sock
MAX_FRAME_BYTES=5242880
READ_TIMEOUT=30s
```

## Streaming

Run the ffmpeg daemon from the repository root and select the camera to stream:

```sh
set -a
source .env
set +a
go run ./services/ffmpeg-streamer/cmd/ffmpeg-streamer --camera-id 17
```

The streamer reads frames from the IPC socket for the configured camera (or composes a merged canvas of all cameras in merge mode), feeds frames to `ffmpeg` over stdin as MJPEG, and publishes an RTMP stream. You can set one direct output URL or one platform stream key:

```sh
STREAM_URL=rtmp://example/live/key
RTMP_URL=rtmp://example/live/key
TWITCH_STREAM_KEY=live_xxx
YOUTUBE_STREAM_KEY=xxxx-xxxx-xxxx-xxxx
```

Useful streamer environment variables:

```sh
IPC_SOCKET_PATH=/tmp/instachron/frames.sock
CAMERA_ID=0
FFMPEG_PATH=ffmpeg
STREAM_FRAME_RATE=10
FFMPEG_RESTART_DELAY=5s
```
