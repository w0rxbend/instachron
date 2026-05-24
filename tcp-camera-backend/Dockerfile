FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /build
COPY go.mod .
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o tcp-camera-backend .

FROM alpine:3.21
COPY --from=builder /build/tcp-camera-backend /usr/local/bin/tcp-camera-backend
EXPOSE 5000
ENTRYPOINT ["tcp-camera-backend"]
