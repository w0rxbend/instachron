FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /build
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o camera-web-restreamer-api .

FROM alpine:3.21
COPY --from=builder /build/camera-web-restreamer-api /usr/local/bin/camera-web-restreamer-api
EXPOSE 8090
ENTRYPOINT ["camera-web-restreamer-api"]
