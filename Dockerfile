FROM golang:1.26-alpine AS builder

WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /out/vnc-recorder ./cmd/vnc-recorder

FROM alpine:3.23

RUN apk add --no-cache ffmpeg ca-certificates tzdata \
  && addgroup -S app \
  && adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/vnc-recorder /usr/local/bin/vnc-recorder

USER app

ENTRYPOINT ["/usr/local/bin/vnc-recorder"]
