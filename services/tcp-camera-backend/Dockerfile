FROM golang:1.22-alpine AS builder
WORKDIR /src

COPY pkg/frameipc/go.mod               pkg/frameipc/
COPY services/tcp-camera-backend/go.mod services/tcp-camera-backend/

RUN go work init ./services/tcp-camera-backend ./pkg/frameipc

COPY pkg/frameipc/                      pkg/frameipc/
COPY services/tcp-camera-backend/       services/tcp-camera-backend/

WORKDIR /src/services/tcp-camera-backend
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/tcp-camera-backend ./cmd/tcp-camera-backend

FROM alpine:3.21
COPY --from=builder /out/tcp-camera-backend /usr/local/bin/tcp-camera-backend
EXPOSE 5000
ENTRYPOINT ["tcp-camera-backend"]
