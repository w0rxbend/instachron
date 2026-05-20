# tcp-camera-backend

Receives JPEG frames from ESP32-CAM devices over a custom binary TCP protocol and writes them atomically to disk.

## Overview

The server accepts raw TCP connections — one per camera or multiple cameras per connection using the JPGD protocol. Each received frame is validated and written atomically to `FRAME_OUTPUT_DIR/<camera-id>/`:

- `frame_<seq>_<ts>.jpg` — named archive file for the current frame
- `current-image.jpeg` — always the latest complete frame (written via atomic rename)

After each write, all previous `frame_*.jpg` files are pruned, keeping exactly one archive file per camera on disk at any time.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|---|---|---|
| `TCP_ADDR` | `0.0.0.0:5000` | Listen address |
| `FRAME_OUTPUT_DIR` | `./frames` | Root directory for frame storage |
| `MAX_FRAME_BYTES` | `5242880` (5 MiB) | Maximum accepted payload size |
| `READ_TIMEOUT` | `30s` | Per-read deadline on each connection |

## Architecture

```mermaid
graph TD
    subgraph "tcp-camera-backend"
        main["main.go\nconfig · signal handling"]
        server["server.go\ntcpFrameServer"]
        protocol["protocol.go\nframe parsing · JPEG validation"]
        storage["storage.go\natomic write · pruning"]
    end

    ESP32_0["ESP32-CAM #0\nJPGS — camera 0"]
    ESP32_N["ESP32-CAM #N\nJPGD — camera N"]
    disk[("FRAME_OUTPUT_DIR\n/0/  /1/  …/N/")]

    ESP32_0 -- "TCP :5000" --> server
    ESP32_N -- "TCP :5000" --> server
    main -- "creates" --> server
    main -- "creates" --> storage
    server -- "uses" --> protocol
    server -- "uses" --> storage
    storage -- "writes" --> disk
```

## Module layout

| File | Responsibility |
|---|---|
| `main.go` | Entry point — reads env config, wires up signal context, creates server and storage |
| `server.go` | TCP listener, per-connection goroutines, frame validation, graceful shutdown |
| `protocol.go` | Binary header parsing for JPGS and JPGD variants, JPEG SOI/EOI check |
| `storage.go` | Atomic file write (temp → sync → rename), `current-image.jpeg` update, old frame pruning |

## Wire protocol

Two frame formats are supported on the same port, distinguished by the first 4 magic bytes.

### JPGS — legacy (single camera, always camera 0)

```
 0       4       8       12      16
 +-------+-------+-------+-------+
 | magic | seq   | size  | ts_ms |   ← 16-byte header
 +-------+-------+-------+-------+
 |        JPEG payload            |   ← size bytes
 +--------------------------------+

magic = 0x4A504753  ("JPGS")
```

### JPGD — multi-camera

```
 0       4       8       12      16      20
 +-------+-------+-------+-------+-------+
 | magic | seq   | size  | ts_ms | camID |   ← 16+4 = 20 bytes
 +-------+-------+-------+-------+-------+
 |          JPEG payload                  |   ← size bytes
 +----------------------------------------+

magic = 0x4A504744  ("JPGD")
```

All fields are big-endian `uint32`.

### Protocol parsing flow

```mermaid
flowchart TD
    A["Read 16 bytes"] --> B{"magic?"}
    B -- "0x4A504753  JPGS" --> C["parseFrameHeader\ncameraID = 0"]
    B -- "0x4A504744  JPGD" --> D["Read 4 more bytes"]
    D --> E["parseFrameHeaderWithCameraID\ncameraID = uint32 big-endian"]
    B -- "anything else" --> F["Error → disconnect"]
    C --> G["Validate payload size\n0 < size ≤ MAX_FRAME_BYTES"]
    E --> G
    G -- "invalid" --> F
    G -- "ok" --> H["Read payload bytes"]
    H --> I{"looksLikeJPEG?\nSOI = 0xFF 0xD8\nEOI = 0xFF 0xD9"}
    I -- "no" --> J["Drop — continue loop"]
    I -- "yes" --> K["writeFrame → storage"]
```

## Frame receive sequence

```mermaid
sequenceDiagram
    participant CAM as ESP32-CAM
    participant SRV as tcpFrameServer
    participant PROT as protocol.go
    participant STOR as frameStorage

    CAM->>SRV: TCP connect
    SRV->>SRV: register conn in map, spawn goroutine

    loop per frame
        CAM->>SRV: 16-byte header
        SRV->>PROT: readFrameHeader()
        alt JPGD magic
            SRV->>CAM: read 4-byte camera ID
        end
        PROT-->>SRV: frameHeader{CameraID, Seq, Size, Ts}
        CAM->>SRV: JPEG payload (Size bytes)
        SRV->>PROT: looksLikeJPEG()
        PROT-->>SRV: bool
        SRV->>STOR: writeFrame(header, payload)
        STOR-->>SRV: path
        SRV->>SRV: log stored frame
    end

    CAM->>SRV: disconnect or read error
    SRV->>SRV: deregister conn, goroutine exits
```

## Storage layout and atomic write

Each camera gets a subdirectory named by its decimal camera ID. There is always exactly one named archive file and one `current-image.jpeg` per camera:

```
FRAME_OUTPUT_DIR/
├── 0/
│   ├── frame_0000000042_0987654321.jpg   ← current archive (only one kept)
│   └── current-image.jpeg                ← latest complete frame
└── 17/
    ├── frame_0000000007_0123456789.jpg
    └── current-image.jpeg
```

`current-image.jpeg` is never a partial file — every write goes through a temp-file-then-rename sequence:

```mermaid
flowchart TD
    A["writeFrame(header, payload)"] --> B["cameraDir = FRAME_OUTPUT_DIR/camID/"]
    B --> C["filename = frame_seq_ts.jpg"]
    C --> D["writeFileAtomic — named archive"]
    D --> D1["CreateTemp .tmp-frame-* in cameraDir"]
    D1 --> D2["Write payload"]
    D2 --> D3["Sync — fsync"]
    D3 --> D4["Close"]
    D4 --> D5["Rename → frame_seq_ts.jpg"]
    D5 --> E["writeFileAtomic — current-image.jpeg"]
    E --> E1["same temp → rename sequence"]
    E1 --> F["pruneOldFrames(cameraDir, filename)"]
    F --> F1["ReadDir cameraDir"]
    F1 --> F2{"each entry"}
    F2 -- "is new archive\nor current-image.jpeg\nor non-frame file" --> F3["skip"]
    F2 -- "old frame_*.jpg" --> F4["os.Remove"]
```

## Graceful shutdown

On `SIGINT` or `SIGTERM` the context is cancelled. Active connections are closed immediately — the server does not wait for the 30-second read timeout to expire:

```mermaid
sequenceDiagram
    participant OS as OS signal
    participant MAIN as main()
    participant SRV as listenAndServe()
    participant CONN as handleConnection() ×N

    OS->>MAIN: SIGINT / SIGTERM
    MAIN->>MAIN: ctx.Cancel()
    SRV->>SRV: shutdown goroutine wakes on ctx.Done()
    SRV->>SRV: listener.Close()
    SRV->>CONN: conn.Close() for every tracked connection
    CONN->>CONN: io.ReadFull returns error → goroutine returns
    CONN->>SRV: wg.Done()
    SRV->>SRV: Accept() errors, ctx.Err() != nil → wg.Wait()
    SRV-->>MAIN: return context.Canceled
    MAIN->>MAIN: errors.Is(context.Canceled) → clean exit
```
