FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /src/services/ffmpeg-streamer
COPY services/ffmpeg-streamer/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/ffmpeg-streamer ./cmd/ffmpeg-streamer

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
COPY --from=builder /out/ffmpeg-streamer /usr/local/bin/ffmpeg-streamer
ENTRYPOINT ["ffmpeg-streamer"]
