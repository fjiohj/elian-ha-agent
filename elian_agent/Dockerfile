ARG BUILD_FROM
FROM golang:1.23-alpine AS builder

ARG ELIAN_AGENT_REF=main
RUN apk add --no-cache ca-certificates git
RUN git clone --depth 1 --branch "${ELIAN_AGENT_REF}" \
    https://github.com/fjiohj/elian-ha-agent.git /src
WORKDIR /src
RUN go mod download
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
    -ldflags "-s -w -X github.com/fjiohj/elian-ha-agent/internal/version.Version=${VERSION} -X github.com/fjiohj/elian-ha-agent/internal/version.Commit=${COMMIT} -X github.com/fjiohj/elian-ha-agent/internal/version.Date=${BUILD_DATE}" \
    -o /out/elian-agent ./cmd/elian-agent

FROM ${BUILD_FROM}
RUN apk add --no-cache ca-certificates tzdata jq
COPY --from=builder /out/elian-agent /usr/local/bin/elian-agent
COPY elian_agent/run.sh /run.sh
RUN chmod a+x /run.sh
CMD ["/run.sh"]
