FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY pkg/cameras/go.mod                            pkg/cameras/
COPY pkg/mjpeg/go.mod                              pkg/mjpeg/
COPY pkg/restream/go.mod                           pkg/restream/
COPY services/camera-web-restream-enhancer-api/go.mod \
     services/camera-web-restream-enhancer-api/go.sum \
     services/camera-web-restream-enhancer-api/

RUN go work init \
    ./services/camera-web-restream-enhancer-api \
    ./pkg/cameras \
    ./pkg/mjpeg \
    ./pkg/restream

WORKDIR /src/services/camera-web-restream-enhancer-api
RUN go mod download

WORKDIR /src
COPY pkg/cameras/                                  pkg/cameras/
COPY pkg/mjpeg/                                    pkg/mjpeg/
COPY pkg/restream/                                 pkg/restream/
COPY services/camera-web-restream-enhancer-api/    services/camera-web-restream-enhancer-api/

WORKDIR /src/services/camera-web-restream-enhancer-api
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restream-enhancer-api ./cmd/camera-web-restream-enhancer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restream-enhancer-api /usr/local/bin/camera-web-restream-enhancer-api
EXPOSE 8091
ENTRYPOINT ["camera-web-restream-enhancer-api"]
