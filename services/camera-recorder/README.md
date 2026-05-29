# camera-recorder

Consumes the internal `streamproto` TCP feed from `camera-web-api` or any restream service and records per-camera H.264 MP4 timelapse segments to storage.

## How It Works

```text
camera-web-api/restream TCP :9001+
  -> camera-recorder TCP upstream client
  -> per-camera bounded queues
  -> timelapse frame sampler
  -> ffmpeg libx264 encoder
  -> storage adapter
  -> HTTP API and Prometheus metrics
```

The default timelapse config records 10 minutes of incoming camera time into about 1 minute of output video:

```text
output_fps = 10
timelapse_factor = 10
keep interval = timelapse_factor / output_fps = 1 second
```

At 10 incoming fps, the recorder keeps roughly one frame per second and feeds those selected frames to ffmpeg as 10 fps output.

## API

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/healthz` | Health check |
| `GET` | `/cameras` | Cameras known from completed files and active recorders |
| `GET` | `/videos?camera_id=0&from=2026-05-29T10:00:00Z&to=...&limit=100` | List recorded files |
| `GET` | `/videos/{cameraID}/{fileName}` | Download/stream an MP4 file |
| `GET` | `/metrics` | Prometheus text metrics |

Each camera directory also gets a `latest.mp4` symlink pointing at the most recently completed segment.

## Configuration

The service reads `CONFIG_FILE` (`config.json` by default) and supports environment overrides for common deployment settings.

| Field / Env | Default | Description |
| --- | --- | --- |
| `http_addr` / `HTTP_ADDR` | `:8094` | HTTP API listen address |
| `upstream_tcp_addr` / `UPSTREAM_TCP_ADDR` | `localhost:9001` | Source `streamproto` TCP address |
| `recording.output_fps` / `OUTPUT_FPS` | `10` | FPS assigned to selected frames in output MP4 |
| `recording.timelapse_factor` / `TIMELAPSE_FACTOR` | `10` | Raw-time speedup factor |
| `recording.segment_raw_duration` / `SEGMENT_RAW_DURATION` | `10m` | Raw camera time per segment |
| `recording.max_file_bytes` / `MAX_FILE_BYTES` | `104857600` | Rotate segment after encoded bytes reach this size |
| `recording.keep_files_per_camera` / `KEEP_FILES_PER_CAMERA` | `144` | Completed MP4 files retained per camera |
| `storage.root_dir` / `STORAGE_ROOT_DIR` | `./recordings` | Local storage root |
| `ffmpeg.path` / `FFMPEG_PATH` | `ffmpeg` | ffmpeg executable |

## Running

```sh
go run ./services/camera-recorder/cmd/camera-recorder
```

Example local override:

```sh
UPSTREAM_TCP_ADDR=localhost:9001 \
STORAGE_ROOT_DIR=/tmp/instachron-recordings \
TIMELAPSE_FACTOR=10 \
go run ./services/camera-recorder/cmd/camera-recorder
```

`ffmpeg` must be installed locally. The Docker image includes it.
