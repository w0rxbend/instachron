FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /build
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o camera-web-api .

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /build/camera-web-api /app/camera-web-api
COPY web/ /app/web/
COPY cameras.json /app/cameras.json
ENV WEB_DIR=/app/web
ENV CAMERA_CONFIG=/app/cameras.json
EXPOSE 8080
ENTRYPOINT ["/app/camera-web-api"]
