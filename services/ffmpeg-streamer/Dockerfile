FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY shared/frameipc/go.mod              shared/frameipc/
COPY services/ffmpeg-streamer/go.mod  services/ffmpeg-streamer/

RUN go work init ./services/ffmpeg-streamer ./shared/frameipc

COPY shared/frameipc/                    shared/frameipc/
COPY services/ffmpeg-streamer/        services/ffmpeg-streamer/

WORKDIR /src/services/ffmpeg-streamer
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/ffmpeg-streamer ./cmd/ffmpeg-streamer

FROM alpine:3.21
RUN apk add --no-cache ffmpeg
COPY --from=builder /out/ffmpeg-streamer /usr/local/bin/ffmpeg-streamer
ENTRYPOINT ["ffmpeg-streamer"]
