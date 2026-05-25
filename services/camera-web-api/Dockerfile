FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /src/services/camera-web-api
COPY services/camera-web-api/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-api ./cmd/camera-web-api

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /out/camera-web-api /app/camera-web-api
COPY --from=builder /src/services/camera-web-api/web/ /app/web/
COPY --from=builder /src/services/camera-web-api/cameras.json /app/cameras.json
ENV WEB_DIR=/app/web
ENV CAMERA_CONFIG=/app/cameras.json
EXPOSE 8080
ENTRYPOINT ["/app/camera-web-api"]
