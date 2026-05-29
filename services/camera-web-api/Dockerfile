FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY shared/cameras/           shared/cameras/
COPY shared/frameipc/          shared/frameipc/
COPY shared/mjpeg/             shared/mjpeg/
COPY shared/restream/          shared/restream/
COPY shared/streamproto/       shared/streamproto/
COPY shared/webui/             shared/webui/
COPY services/camera-web-api/  services/camera-web-api/
WORKDIR /src/services/camera-web-api
RUN GOWORK=off CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-api ./cmd/camera-web-api

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/camera-web-api                       /app/camera-web-api
COPY --from=builder /src/services/camera-web-api/cameras.json /app/cameras.json
ENV CAMERA_CONFIG=/app/cameras.json
EXPOSE 8080
ENTRYPOINT ["/app/camera-web-api"]
