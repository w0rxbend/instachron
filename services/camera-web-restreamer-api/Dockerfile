FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /src/services/camera-web-restreamer-api
COPY services/camera-web-restreamer-api/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restreamer-api ./cmd/camera-web-restreamer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restreamer-api /usr/local/bin/camera-web-restreamer-api
EXPOSE 8090
ENTRYPOINT ["camera-web-restreamer-api"]
