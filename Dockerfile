FROM golang:1.23-alpine AS builder

WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
    -ldflags "-s -w -X github.com/fjiohj/elian-ha-agent/internal/version.Version=${VERSION} -X github.com/fjiohj/elian-ha-agent/internal/version.Commit=${COMMIT} -X github.com/fjiohj/elian-ha-agent/internal/version.Date=${BUILD_DATE}" \
    -o /out/elian-agent ./cmd/elian-agent

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
RUN addgroup -S elian \
    && adduser -S -G elian elian \
    && mkdir -p /data \
    && chown -R elian:elian /data
COPY --from=builder /out/elian-agent /usr/local/bin/elian-agent
ENV ELIAN_STATE_DIR=/data
VOLUME ["/data"]
USER elian
ENTRYPOINT ["/usr/local/bin/elian-agent"]
