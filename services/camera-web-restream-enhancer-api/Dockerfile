FROM golang:1.22-alpine AS builder
ENV GOWORK=off
WORKDIR /src/services/camera-web-restream-enhancer-api
COPY services/camera-web-restream-enhancer-api/ ./
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/camera-web-restream-enhancer-api ./cmd/camera-web-restream-enhancer-api

FROM alpine:3.21
COPY --from=builder /out/camera-web-restream-enhancer-api /usr/local/bin/camera-web-restream-enhancer-api
EXPOSE 8091
ENTRYPOINT ["camera-web-restream-enhancer-api"]
