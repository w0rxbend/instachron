# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY shared/cameras/go.mod                          shared/cameras/
COPY shared/mjpeg/go.mod                            shared/mjpeg/
COPY shared/restream/go.mod                         shared/restream/
COPY shared/streamproto/go.mod                      shared/streamproto/
COPY shared/webui/go.mod                            shared/webui/
COPY services/camera-web-restream-upscaler-api/go.mod \
     services/camera-web-restream-upscaler-api/go.sum \
     services/camera-web-restream-upscaler-api/
WORKDIR /src/services/camera-web-restream-upscaler-api
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/cameras/                                shared/cameras/
COPY shared/mjpeg/                                  shared/mjpeg/
COPY shared/restream/                               shared/restream/
COPY shared/streamproto/                            shared/streamproto/
COPY shared/webui/                                  shared/webui/
COPY services/camera-web-restream-upscaler-api/       services/camera-web-restream-upscaler-api/
WORKDIR /src/services/camera-web-restream-upscaler-api
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restream-upscaler-api ./cmd/camera-web-restream-upscaler-api

# ── Runtime stage ─────────────────────────────────────────────────────────────
# Pure static binary — no OS needed.
FROM scratch
COPY --from=builder /out/camera-web-restream-upscaler-api /camera-web-restream-upscaler-api

EXPOSE 8092
ENTRYPOINT ["/camera-web-restream-upscaler-api"]
