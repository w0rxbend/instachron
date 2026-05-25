FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY shared/cameras/go.mod               shared/cameras/
COPY shared/frameipc/go.mod              shared/frameipc/
COPY shared/mjpeg/go.mod                 shared/mjpeg/
COPY services/camera-web-api/go.mod   services/camera-web-api/

RUN go work init ./services/camera-web-api ./shared/cameras ./shared/frameipc ./shared/mjpeg

COPY shared/cameras/                     shared/cameras/
COPY shared/frameipc/                    shared/frameipc/
COPY shared/mjpeg/                       shared/mjpeg/
COPY services/camera-web-api/         services/camera-web-api/

WORKDIR /src/services/camera-web-api
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-api ./cmd/camera-web-api

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/camera-web-api                      /app/camera-web-api
COPY --from=builder /src/services/camera-web-api/web/        /app/web/
COPY --from=builder /src/services/camera-web-api/cameras.json /app/cameras.json
ENV WEB_DIR=/app/web
ENV CAMERA_CONFIG=/app/cameras.json
EXPOSE 8080
ENTRYPOINT ["/app/camera-web-api"]
