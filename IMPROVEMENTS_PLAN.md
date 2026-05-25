## Implementation plan for agent

### Target architecture

```text
camera-web-api
  exposes:
    - TCP framed JPEG stream
    - HTTP multipart/x-mixed-replace

restreamer* / restream*
  reads:
    - upstream TCP framed JPEG stream

  exposes:
    - downstream TCP framed JPEG stream
    - public HTTP multipart/x-mixed-replace
```

Internal transport is always:

```text
TCP framed JPEG
```

External compatibility API is:

```text
HTTP multipart/x-mixed-replace
```

---

# Phase 1 — Refactor source app

## 1. Add internal frame model

Create shared package:

```text
/shared/streamproto
```

Frame header:

```text
Magic:       4 bytes  "MJPG"
Version:     uint8    1
Flags:       uint8
HeaderLen:   uint16
TimestampNs: uint64
CameraID:    uint32
Sequence:    uint64
PayloadLen:  uint32
```

Payload:

```text
raw JPEG bytes
```

Rules:

```text
PayloadLen > 0
PayloadLen <= maxFrameSize
Magic must match
Version must match
TimestampNs from producer
CameraID from producer
Sequence increments per frame
```

---

## 2. Implement encoder/decoder

Required API:

```go
type Frame struct {
    Timestamp time.Time
    Sequence  uint64
    CameraID  uint32
    Payload   []byte
}

type Writer struct {
    w io.Writer
}

func (w *Writer) WriteFrame(ctx context.Context, f Frame) error

type Reader struct {
    r io.Reader
}

func (r *Reader) ReadFrame(ctx context.Context) (Frame, error)
```

Must use:

```go
io.ReadFull
binary.BigEndian
maxFrameSize validation
```

No JPEG decoding in protocol layer.

---

## 3. Add TCP server to source app

Config:

```yaml
tcp:
  enabled: true
  listen_addr: "0.0.0.0:9001"
  max_clients: 64
  write_timeout: 2s
```

Behavior:

```text
source JPEG frames
    ↓
latest-frame fanout
    ↓
TCP clients receive framed JPEG
```

Each TCP client:

```text
bounded queue size = 1 or 2
drop old frame if slow
disconnect if write timeout
```

---

## 4. Keep HTTP multipart endpoint

Source app should still expose HTTP multipart/x-mixed-replace for API HTTP clients.

But internally it should reuse the same frame bus as TCP. 

BUT KEEP IN MIND THAT THIS TCP PROTOCOL IS NOT FOR CAMERA CLIENTS, CAMERA CLIENT uses `shared/frameipc`,

WE SHOULD INTRODUCE NEW PROTOCOL FOR INTERNAL PROXY-TO-PROXY COMMUNICATION. EVENT if IT LOOKS SIMILAR TO `frameipc`, IT SHOULD BE INDEPENDENT AND NOT TIED TO CAMERA CLIENTS.

---

# Phase 2 — Shared frame bus

Implement a reusable broadcaster:

```go
type Broadcaster struct {
    subscribers map[chan Frame]struct{}
}

func Subscribe() (<-chan Frame, unsubscribe func())
func Publish(Frame)
```

Rules:

```text
non-blocking publish
per-subscriber queue size 1 or 2
drop old frame on slow subscriber
store latest frame
send latest frame immediately on subscribe
```

Important:

```text
slow client must never block upstream reader
```

---

# Phase 3 — Add/extend proxy apps: restreamer* / restream*

## 1. Upstream reader

Config:

```yaml
upstream:
  tcp_addr: "source-app:9001"
  reconnect:
    min_backoff: 500ms
    max_backoff: 10s
```

Behavior:

```text
connect to upstream TCP
read framed JPEG frames
validate sequence/timestamp
publish to local broadcaster
reconnect on EOF/error
```

No multipart upstream support for proxies.

Mandatory rule:

```text
restreamer/restream always consume upstream via TCP framed JPEG
```

---

## 2. Expose downstream TCP

Config:

```yaml
tcp:
  enabled: true
  listen_addr: "0.0.0.0:9002"
  max_clients: 64
```

Behavior:

```text
local broadcaster
    ↓
TCP framed JPEG writer
    ↓
downstream restream/restreamer instances
```

This allows stacking:

```text
source-app:9001
    ↓ TCP framed JPEG
restreamer:9002
    ↓ TCP framed JPEG
restream:9003
    ↓ TCP framed JPEG
clients via HTTP
```

---

## 3. Expose public HTTP multipart

Endpoint the same as source app for compatibility:


# Phase 4 — Backpressure and dropping

Use this policy everywhere:

```text
keep latest frame
drop old frame
never block producer
```

For each subscriber:

```go
select {
case ch <- frame:
default:
    <-ch
    ch <- frame
}
```

For TCP writers:

```text
if client cannot receive within write_timeout:
    disconnect client

and make it resilient to transient network errors, connection resets, retries, etc.
```

For HTTP writers:

```text
if ResponseWriter write/flush blocks or fails:
    disconnect client
and make it resilient to transient network errors, client disconnects, retries, etc.
```

---

# Phase 5 — Config model

Example config:

```yaml
app:
  name: restreamer

upstream:
  protocol: tcp-framed-jpeg
  addr: "source-app:9001"

tcp:
  enabled: true
  listen_addr: "0.0.0.0:9002"
  max_clients: 64
  max_frame_size: 10485760
  write_timeout: 2s

http:
  enabled: true
  listen_addr: "0.0.0.0:8080"
  stream_path: "/stream.mjpeg"

stream:
  subscriber_queue_size: 2
  send_latest_on_subscribe: true
```

---

# Phase 6 — Metrics

Expose:

```text
frames_received_total
frames_published_total
frames_dropped_total
tcp_clients_current
http_clients_current
upstream_connected
upstream_reconnects_total
upstream_read_errors_total
tcp_write_errors_total
http_write_errors_total
latest_frame_age_seconds
latest_sequence
bytes_in_total
bytes_out_total
```


# Phase 8 — Final runtime topology

```text
[source-app]
  :9001 TCP framed JPEG
  :8080 HTTP multipart

      ↓ TCP framed JPEG

[restreamer]
  reads :9001
  exposes :9002 TCP framed JPEG
  exposes :8081 HTTP multipart

      ↓ TCP framed JPEG

[restream]
  reads :9002
  exposes :9003 TCP framed JPEG
  exposes :8082 HTTP multipart
```

The key invariant:

```text
proxy-to-proxy transport = TCP framed JPEG
client-facing transport = HTTP multipart/x-mixed-replace
```

So proxies can be stacked arbitrarily without breaking internal transport, and clients can still consume via HTTP multipart if they want.