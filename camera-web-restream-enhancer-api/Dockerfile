FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /build
COPY go.mod go.sum .
RUN go mod download
COPY *.go .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o camera-web-restream-enhancer-api .

FROM alpine:3.21
COPY --from=builder /build/camera-web-restream-enhancer-api /usr/local/bin/camera-web-restream-enhancer-api
EXPOSE 8091
ENTRYPOINT ["camera-web-restream-enhancer-api"]
