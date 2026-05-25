# Instachron Go Restructure Plan

## Goal

Refactor the Go codebase into a maintainable production monorepo without importing ceremonial architecture. The target is:

- Clear service ownership.
- Shared protocol and streaming code extracted once.
- Small `main` packages that only parse config, wire dependencies, and start the process.
- Private implementation under `internal`.
- Tests around protocol, IPC, stream fan-out, image processing, and service startup wiring.
- Docker and Compose paths that still make each service deployable independently.

## Research Notes

The useful Go guidance is simpler than most internet folder-layout lists:

- Go organizes code by packages, not by Java-style layers. Package names should be short, clear, lowercase, and meaningful from the caller's point of view. Source: https://go.dev/blog/package-names
- For server projects, Go recommends keeping implementation packages in `internal` because servers usually do not expose public importable APIs. Source: https://go.dev/doc/modules/layout
- `cmd/<binary>/main.go` is a common convention when a repository has several commands or a mix of commands and packages. Source: https://go.dev/doc/modules/layout
- `pkg` is not magic and should not be used as a dumping ground. In this repo, `pkg/` means first-party shared modules that have real consumers across services.
- If a server repository grows code that should be shared by multiple projects, the Go docs recommend splitting that shared code into separate modules. Source: https://go.dev/doc/modules/layout

Conclusion: for this repo, the best fit is **monorepo + service modules + internal service packages + explicitly shared `pkg` modules**. Clean/hexagonal/onion ideas can inform dependency direction, but the folder names should reflect this camera streaming domain.

## Workspace and Module Policy

Keep `go.work` as a first-class part of the repository. This project should remain a multi-module Go workspace because each runtime service is independently deployable and can keep its own dependency graph.

Canonical GitHub repository:

```text
github.com/w0rxbend/instachron
```

Module paths after the restructure should use that repository root:

```text
github.com/w0rxbend/instachron/services/tcp-camera-backend
github.com/w0rxbend/instachron/services/camera-web-api
github.com/w0rxbend/instachron/services/camera-web-restreamer-api
github.com/w0rxbend/instachron/services/camera-web-restream-enhancer-api
github.com/w0rxbend/instachron/services/camera-web-restream-fsrcnn-api
github.com/w0rxbend/instachron/services/ffmpeg-streamer
github.com/w0rxbend/instachron/pkg/frameipc
github.com/w0rxbend/instachron/pkg/mjpeg
github.com/w0rxbend/instachron/pkg/cameras
github.com/w0rxbend/instachron/pkg/restream
```

Do not collapse this into one root `go.mod` unless deployment and dependency ownership change later. The root should have `go.work`, docs, Compose, dashboard files, and orchestration; Go dependency ownership stays at each module.

Important workspace rule: `go.work` is for local development and repository-level CI convenience. Each module still needs a valid `go.mod` with explicit `require` entries for any `pkg/*` modules it imports. Do not rely on hidden local state.

For first-party `pkg/*` imports during active development, use a placeholder requirement such as:

```text
require github.com/w0rxbend/instachron/pkg/frameipc v0.0.0
```

The workspace then resolves that module to `./pkg/frameipc`. When a shared module is intentionally released for use outside this workspace, tag it using Go's submodule tag convention, for example `pkg/frameipc/v0.1.0`, and update service requirements deliberately.

## Current State

The repository is already a monorepo with a Go workspace:

```text
go.work
camera-web-api/
camera-web-restreamer-api/
camera-web-restream-enhancer-api/
camera-web-restream-fsrcnn-api/
tcp-camera-backend/
ffmpeg-streamer/
dashboard/
docker-compose.yml
```

Strengths:

- Each runtime process is already isolated as its own Go module.
- Docker images are independently buildable.
- The code is still small enough to migrate safely in phases.
- `go.work` already expresses local multi-module development.

Problems to fix before the project gets larger:

- Every Go service is currently `package main`, which makes logic harder to reuse and test outside the process entry point.
- `hub`, `handler`, `discovery`, and `upstream` code repeats across restream services.
- IPC frame message reading/writing is duplicated across `tcp-camera-backend`, `camera-web-api`, and `ffmpeg-streamer`.
- Environment parsing is repeated in several `main.go` files.
- Dockerfiles copy `*.go`, which will break as soon as code moves under subdirectories.
- Per-service Docker build contexts will not see `pkg/*` once shared modules are introduced unless Compose builds from the repository root or the Dockerfiles explicitly receive those modules.
- README still describes an older two-module shape and should be updated after the restructure.
- There are only a few tests today, mostly protocol and image composition tests.

## Recommended Target Structure

Keep services as separately deployable modules, move them under `services/`, and introduce shared modules under `pkg/` only where reuse is real.

```text
instachron/
|-- go.work
|-- docker-compose.yml
|-- services/
|   |-- tcp-camera-backend/
|   |   |-- cmd/tcp-camera-backend/main.go
|   |   |-- internal/config/
|   |   |-- internal/server/
|   |   `-- internal/publisher/
|   |-- camera-web-api/
|   |   |-- cmd/camera-web-api/main.go
|   |   |-- internal/config/
|   |   |-- internal/httpapi/
|   |   |-- internal/camera/
|   |   |-- internal/ipcclient/
|   |   `-- internal/rotation/
|   |-- camera-web-restreamer-api/
|   |   |-- cmd/camera-web-restreamer-api/main.go
|   |   |-- internal/config/
|   |   `-- internal/app/
|   |-- camera-web-restream-enhancer-api/
|   |   |-- cmd/camera-web-restream-enhancer-api/main.go
|   |   |-- internal/config/
|   |   |-- internal/app/
|   |   `-- internal/enhance/
|   |-- camera-web-restream-fsrcnn-api/
|   |   |-- cmd/camera-web-restream-fsrcnn-api/main.go
|   |   |-- internal/config/
|   |   |-- internal/app/
|   |   |-- internal/fsrcnn/
|   |   |-- internal/imageio/
|   |   |-- internal/pipeline/
|   |   `-- internal/metrics/
|   `-- ffmpeg-streamer/
|       |-- cmd/ffmpeg-streamer/main.go
|       |-- internal/config/
|       |-- internal/ffmpeg/
|       |-- internal/ipcclient/
|       `-- internal/compose/
|-- pkg/
|   |-- frameipc/
|   |   |-- go.mod
|   |   `-- frameipc.go
|   |-- mjpeg/
|   |   |-- go.mod
|   |   `-- mjpeg.go
|   |-- cameras/
|   |   |-- go.mod
|   |   `-- cameras.go
|   `-- restream/
|       |-- go.mod
|       |-- discovery.go
|       |-- hub.go
|       `-- upstream.go
`-- dashboard/
```

### Package Responsibilities

`pkg/frameipc`

- Owns the Unix socket message schema used between the TCP backend and consumers.
- Provides `Message`, `Writer`, and `Reader` helpers.
- Replaces duplicated IPC structs and binary write/read code.

`pkg/mjpeg`

- Owns multipart MJPEG frame writing.
- Replaces duplicated `mjpegBoundary` and `writeMJPEGFrame`.

`pkg/cameras`

- Owns shared camera DTOs used by web and restream APIs.
- Keep it intentionally small: identifiers, index, rotation, online state, and JSON shape.

`pkg/restream`

- Owns the shared pattern used by restreamer, enhancer, and FSRCNN services:
  origin discovery, upstream MJPEG reading, per-camera hub, liveness, snapshots, and streaming.
- Exposes extension points such as `Processor interface { Process(ctx context.Context, cameraID string, jpeg []byte) ([]byte, error) }`.
- Plain restreamer uses a no-op processor.
- Enhancer and FSRCNN provide processors from their service-specific `internal` packages.

Service `internal/*`

- Owns code that is not meant to be imported outside that service.
- Keeps external dependencies at the edges: HTTP, ffmpeg process control, ONNX runtime, image manipulation, environment config, sockets.

`cmd/<service>/main.go`

- Does only process-level work:
  load config, create logger, create dependencies, start goroutines, handle shutdown.

### Docker Build Policy

After shared `pkg/*` modules exist, prefer repository-root Docker build contexts:

```yaml
camera-web-api:
  build:
    context: .
    dockerfile: services/camera-web-api/Dockerfile
```

This lets Dockerfiles copy `go.work`, the service module, and any required `pkg/*` modules in one build context. The alternative is keeping per-service build contexts and fetching shared modules from GitHub during builds, but that is slower, more fragile during local development, and awkward before changes are pushed.

If the committed `go.work` lists all workspace modules, the Docker build must either:

- copy every module directory named by `go.work`, or
- create a minimal build-time workspace containing only the service and libraries needed by that image.

The minimal build-time workspace is usually cleaner for image builds.

Recommended Dockerfile pattern after the move:

```Dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY services/camera-web-api/go.mod services/camera-web-api/go.mod
COPY pkg/frameipc/go.mod pkg/frameipc/go.mod
COPY pkg/mjpeg/go.mod pkg/mjpeg/go.mod
RUN go work init ./services/camera-web-api ./pkg/frameipc ./pkg/mjpeg
COPY services/camera-web-api services/camera-web-api
COPY pkg/frameipc pkg/frameipc
COPY pkg/mjpeg pkg/mjpeg
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-api ./services/camera-web-api/cmd/camera-web-api
```

For services with no shared library imports yet, this pattern is still acceptable because it keeps all service Dockerfiles consistent.

## Why Not Pure Clean/Hexagonal/Onion

Those architectures are useful when business rules are complex and stable enough to justify strict dependency boundaries. Instachron is mostly a streaming, protocol, and image-processing system. The highest-risk code is not an enterprise entity model; it is:

- TCP frame parsing.
- IPC fan-out.
- MJPEG stream handling.
- Camera state and liveness.
- Image enhancement/upscaling pipelines.
- ffmpeg process lifecycle.

So the restructure should be package-driven and domain-driven, not layer-folder-driven. Packages named `handler`, `service`, `repository`, and `model` would mostly hide the real concepts.

## Migration Plan

### Phase 0: Safety Baseline

1. Record current behavior:
   - `docker compose config`
   - current service ports and environment variables
   - current profile behavior for `streaming` and `fsrcnn`
2. Ensure Go is available in the development environment or CI runner.
3. Run, or add a CI job to run module-aware checks:
   - `go work sync`
   - `go test ./...` from each workspace module directory
   - `docker compose build`
4. Do not rename services and move files in the same commit as behavior changes.

Current blocker found during planning: `go` is not installed in this local shell, so tests cannot currently be run here.

Suggested temporary shell loop before moving modules under `services/`:

```sh
for mod in \
  tcp-camera-backend \
  camera-web-api \
  camera-web-restreamer-api \
  camera-web-restream-enhancer-api \
  camera-web-restream-fsrcnn-api \
  ffmpeg-streamer
do
  (cd "$mod" && go test ./...)
done
```

Suggested temporary shell loop after the target structure exists:

```sh
for mod in \
  services/tcp-camera-backend \
  services/camera-web-api \
  services/camera-web-restreamer-api \
  services/camera-web-restream-enhancer-api \
  services/camera-web-restream-fsrcnn-api \
  services/ffmpeg-streamer \
  pkg/frameipc \
  pkg/mjpeg \
  pkg/cameras \
  pkg/restream
do
  (cd "$mod" && go test ./...)
done
```

### Phase 1: Move Services Under `services/`

1. Move each service directory into `services/<service>/`.
2. Update each service `go.mod` module path to match its new repository subdirectory, for example `github.com/w0rxbend/instachron/services/camera-web-api`.
3. Update `go.work` and include service modules plus any new `pkg/*` modules:

```text
use (
    ./services/camera-web-api
    ./services/camera-web-restreamer-api
    ./services/camera-web-restream-enhancer-api
    ./services/camera-web-restream-fsrcnn-api
    ./services/ffmpeg-streamer
    ./services/tcp-camera-backend
    ./pkg/frameipc
    ./pkg/mjpeg
    ./pkg/cameras
    ./pkg/restream
)
```

4. Update `docker-compose.yml` build contexts and volume paths.
5. Update Dockerfiles to use repository-root build contexts where services import shared `pkg/*` modules.
6. Rename `camera-web-restream-FSRCNN-api` to `camera-web-restream-fsrcnn-api` for lowercase path consistency. Also use the lowercase Compose service name because Podman/Docker image tags require lowercase repository names.

Verification:

- `docker compose config`
- `go list ./...` inside each service module
- `docker compose build tcp-camera-backend camera-web-api camera-web-restreamer-api`

### Phase 2: Introduce `cmd` and `internal` Inside Each Service

Do this service by service. Start with the smallest low-risk services.

Suggested order:

1. `tcp-camera-backend`
2. `camera-web-api`
3. `ffmpeg-streamer`
4. `camera-web-restreamer-api`
5. `camera-web-restream-enhancer-api`
6. `camera-web-restream-fsrcnn-api`

For each service:

1. Move process entry points to `cmd/<service>/main.go`.
2. Move application wiring into `internal/app/run.go`.
3. Move config parsing into `internal/config`.
4. Move domain code into named packages, not generic layer names.
5. Keep exported identifiers minimal.
6. Update Dockerfile build command:

```sh
go build -ldflags="-s -w" -o /out/<service> ./cmd/<service>
```

Verification:

- Existing tests still pass.
- New package imports do not form cycles.
- Docker image still starts with the same environment variables.

### Phase 3: Extract Shared IPC

Create `pkg/frameipc` as its own Go module.

Initial API shape:

```go
package frameipc

type Message struct {
    CameraID uint32
    JPEG     []byte
    Offline  bool
}

type Reader struct {}
type Writer struct {}
```

Move shared behavior from:

- `tcp-camera-backend/publisher.go`
- `camera-web-api/socket_reader.go`
- `ffmpeg-streamer/ipc.go`

Rules:

- The binary wire format must remain backward compatible.
- Add `require github.com/w0rxbend/instachron/pkg/frameipc v0.0.0` to importing service modules while the service is built inside this workspace. During local development and CI, `go.work` supplies the local checkout. If the shared module is later released outside the workspace, replace `v0.0.0` with a real submodule tag.
- Add tests for write/read round trips.
- Add tests for offline messages.
- Add tests for malformed payloads and short reads.

Verification:

- `cd pkg/frameipc && go test ./...`
- Service tests for `tcp-camera-backend`, `camera-web-api`, and `ffmpeg-streamer`

### Phase 4: Extract Shared MJPEG HTTP Helpers

Create `pkg/mjpeg`.

Move shared behavior from:

- `camera-web-api/handler.go`
- `camera-web-restreamer-api/handler.go`
- `camera-web-restream-enhancer-api/handler.go`
- `camera-web-restream-fsrcnn-api/handler.go`

Suggested package responsibilities:

- Boundary constant.
- HTTP content type helper.
- Frame writer.
- Optional stream loop helper only if it does not force awkward dependencies.

Rules:

- Keep HTTP handler ownership in services. `pkg/mjpeg` should write frames and headers, not own routes.
- Add explicit `require` entries for services that import it.

Verification:

- Unit test generated multipart frame headers.
- Smoke test `/stream` endpoints manually or through integration tests.

### Phase 5: Extract Shared Camera Stream State

Create `pkg/restream` after IPC and MJPEG are stable.

Move common code from restream services:

- `hub.go`
- `discovery.go`
- `upstream.go`
- HTTP camera DTO integration

Design target:

```go
type Processor interface {
    Process(ctx context.Context, cameraID string, jpeg []byte) ([]byte, error)
}

type NoopProcessor struct{}
```

The restream services become thin compositions:

- `camera-web-restreamer-api`: `restream.NoopProcessor`
- `camera-web-restream-enhancer-api`: `internal/enhance.Processor`
- `camera-web-restream-fsrcnn-api`: `internal/pipeline.Processor`

Verification:

- One shared test suite for liveness, hub subscription, snapshot, and discovery update behavior.
- Service-specific tests only cover processor wiring and config.

Do not extract this package too early. If the three restream services still differ in meaningful ways after IPC and MJPEG extraction, keep duplicated service-local code a little longer and extract only the smaller stable pieces.

### Phase 6: Strengthen Service-Specific Packages

`tcp-camera-backend`

- `internal/protocol`: frame header parsing and JPEG validation.
- `internal/server`: TCP accept loop and connection handling.
- `internal/publisher`: socket fan-out using `pkg/frameipc`.
- `internal/config`: environment configuration loading.

`camera-web-api`

- `internal/ipcclient`: frame socket reader using `pkg/frameipc`.
- `internal/camera`: live camera hub and state.
- `internal/rotation`: config and JPEG rotation.
- `internal/httpapi`: routes and handlers.

`ffmpeg-streamer`

- `internal/config`: env/flag parsing.
- `internal/ffmpeg`: command args, process lifecycle, restart behavior.
- `internal/compose`: merged canvas and image fitting.
- `internal/ipcclient`: camera frame cache using `pkg/frameipc`.

`camera-web-restream-enhancer-api`

- `internal/enhance`: image enhancement configuration and processor.

`camera-web-restream-fsrcnn-api`

- `internal/fsrcnn`: ONNX session lifecycle.
- `internal/imageio`: YCbCr split/merge, encode/decode, resolution cap.
- `internal/pipeline`: queue, workers, warmup.
- `internal/metrics`: counters and reporter.

### Phase 7: Update Docs and Operational Files

1. Update root README to describe all services, not just the old two-module setup.
2. Add a short architecture document under `docs/architecture.md`.
3. Add `make` or `just` commands only if the team wants a stable command surface:
   - `test`
   - `build`
   - `compose-build`
   - `compose-up-core`
4. Add CI checks:
   - `go test` for all workspace modules
   - `go work sync` followed by a clean diff check
   - `gofmt`
   - `go vet`
   - Docker build for core services

## Import Rules

- Service code may import its own `internal` packages.
- Service code may import `pkg/*` modules.
- `pkg/*` modules must not import service packages.
- `pkg/*` modules must have stable, small public APIs.
- A service `go.mod` must explicitly `require` every `pkg/*` module it imports.
- Keep `replace` directives out of committed service `go.mod` files for first-party modules; use `go.work` for local development instead.
- Avoid packages named only `handler`, `service`, `models`, or `utils` unless the package name makes sense in client code.
- Prefer package names like `frameipc`, `mjpeg`, `rotation`, `pipeline`, `enhance`, `compose`, `publisher`, and `protocol`.

## Testing Plan

Add focused tests before and during moves:

- `frameipc`: encode/decode, offline message, malformed input.
- `protocol`: frame headers, max size, JPEG marker validation.
- `mjpeg`: multipart headers and frame body.
- `camera` or `restream`: subscribe/unsubscribe, latest frame, stale/offline transitions.
- `rotation`: degree normalization and image rotation dimensions.
- `enhance`: dark-frame adjustment boundaries.
- `pipeline`: queue drop behavior, worker shutdown, metrics recording.
- `ffmpeg`: generated args and config validation.

## Rollout Strategy

Use small PRs or commits:

1. Move folders under `services/` and update paths only.
2. Add `cmd` and `internal` to one service at a time.
3. Extract `pkg/frameipc`.
4. Extract `pkg/mjpeg`.
5. Extract `pkg/restream`.
6. Update docs and CI.

Each step should preserve the public runtime contract:

- Same ports.
- Same environment variables.
- Same Compose service names unless intentionally changed.
- Same stream URLs and endpoint paths.
- Same IPC socket path.

## Final Recommendation

Do not adopt a generic layered template. Adopt a Go-native monorepo structure:

- `services/` for deployable processes.
- `cmd/<service>` for process entry points.
- `internal/` for private implementation.
- `pkg/` only for shared modules with real consumers.
- Domain package names based on what Instachron actually does: `frameipc`, `mjpeg`, `protocol`, `rotation`, `restream`, `pipeline`, `enhance`, `compose`.

This gives the project production-grade boundaries without making every future feature pay an architecture tax.
