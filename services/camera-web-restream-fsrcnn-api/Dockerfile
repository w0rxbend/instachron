# ── Build stage ──────────────────────────────────────────────────────────────
# Use a glibc-based image so the CGO binary is linked against glibc, matching
# the runtime image (debian:bookworm-slim).
FROM golang:1.22-bookworm AS builder
WORKDIR /src

COPY shared/cameras/go.mod                           shared/cameras/
COPY shared/mjpeg/go.mod                             shared/mjpeg/
COPY shared/restream/go.mod                          shared/restream/
COPY services/camera-web-restream-fsrcnn-api/go.mod \
     services/camera-web-restream-fsrcnn-api/go.sum \
     services/camera-web-restream-fsrcnn-api/

RUN go work init \
    ./services/camera-web-restream-fsrcnn-api \
    ./shared/cameras \
    ./shared/mjpeg \
    ./shared/restream

WORKDIR /src/services/camera-web-restream-fsrcnn-api
RUN go mod download

WORKDIR /src
COPY shared/cameras/                                 shared/cameras/
COPY shared/mjpeg/                                   shared/mjpeg/
COPY shared/restream/                                shared/restream/
COPY services/camera-web-restream-fsrcnn-api/     services/camera-web-restream-fsrcnn-api/

WORKDIR /src/services/camera-web-restream-fsrcnn-api
RUN go build -ldflags="-s -w" -o /out/camera-web-restream-fsrcnn-api ./cmd/camera-web-restream-fsrcnn-api

# ── ONNX Runtime fetch stage ──────────────────────────────────────────────────
# Downloads and unpacks the prebuilt ORT CPU library so the final image does
# not need wget at runtime. Override ORT_VERSION to switch library versions.
FROM debian:bookworm-slim AS ort-fetcher
ARG ORT_VERSION=1.20.1
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN wget -qO /tmp/ort.tgz \
      "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-x64-${ORT_VERSION}.tgz" \
    && tar -xzf /tmp/ort.tgz -C /tmp \
    && mkdir /ort \
    && cp -P /tmp/onnxruntime-linux-x64-${ORT_VERSION}/lib/libonnxruntime.so* /ort/ \
    && rm -rf /tmp/ort.tgz "/tmp/onnxruntime-linux-x64-${ORT_VERSION}"

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim
# libgomp1 is required by ORT for multi-threaded CPU inference.
RUN apt-get update && apt-get install -y --no-install-recommends libgomp1 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=ort-fetcher /ort/ /usr/local/lib/
RUN ldconfig

COPY --from=builder /out/camera-web-restream-fsrcnn-api /usr/local/bin/camera-web-restream-fsrcnn-api

RUN mkdir -p /models

# onnxruntime_go calls SetSharedLibraryPath when ONNX_LIB_PATH is non-empty.
# Pointing it to the versioned .so avoids relying on the ldconfig symlink.
ENV ONNX_LIB_PATH=/usr/local/lib/libonnxruntime.so
ENV MODEL_PATH=/models/fsrcnn_x2.onnx

EXPOSE 8092
ENTRYPOINT ["camera-web-restream-fsrcnn-api"]
