FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY shared/streamproto/go.mod                      shared/streamproto/
COPY shared/cameras/go.mod                          shared/cameras/
COPY shared/restream/go.mod                         shared/restream/
COPY services/camera-recorder/go.mod                services/camera-recorder/
WORKDIR /src/services/camera-recorder
RUN GOWORK=off go mod download

WORKDIR /src
COPY shared/streamproto/                            shared/streamproto/
COPY shared/cameras/                                shared/cameras/
COPY shared/restream/                               shared/restream/
COPY services/camera-recorder/                      services/camera-recorder/
WORKDIR /src/services/camera-recorder
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-recorder ./cmd/camera-recorder

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
WORKDIR /app
COPY --from=builder /out/camera-recorder /usr/local/bin/camera-recorder
COPY --from=builder /src/services/camera-recorder/config.json /app/config.json
ENV CONFIG_FILE=/app/config.json
EXPOSE 8094
VOLUME ["/recordings"]
ENTRYPOINT ["camera-recorder"]
