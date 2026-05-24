FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /build
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ffmpeg-streamer .

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
COPY --from=builder /build/ffmpeg-streamer /usr/local/bin/ffmpeg-streamer
ENTRYPOINT ["ffmpeg-streamer"]
