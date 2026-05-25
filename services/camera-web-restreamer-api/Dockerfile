FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY shared/cameras/go.mod                       shared/cameras/
COPY shared/mjpeg/go.mod                         shared/mjpeg/
COPY shared/restream/go.mod                      shared/restream/
COPY services/camera-web-restreamer-api/go.mod services/camera-web-restreamer-api/

RUN go work init \
    ./services/camera-web-restreamer-api \
    ./shared/cameras \
    ./shared/mjpeg \
    ./shared/restream

COPY shared/cameras/                             shared/cameras/
COPY shared/mjpeg/                               shared/mjpeg/
COPY shared/restream/                            shared/restream/
COPY services/camera-web-restreamer-api/      services/camera-web-restreamer-api/

WORKDIR /src/services/camera-web-restreamer-api
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restreamer-api ./cmd/camera-web-restreamer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restreamer-api /usr/local/bin/camera-web-restreamer-api
EXPOSE 8090
ENTRYPOINT ["camera-web-restreamer-api"]
