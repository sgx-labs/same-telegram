# --- Build stage ---
FROM golang:1.25-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-s -w -X main.version=fly" \
    -o /usr/local/bin/same-telegram \
    ./cmd/same-telegram/

# --- Runtime stage ---
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN groupadd -r samebot && useradd -r -g samebot -d /data -s /sbin/nologin samebot

COPY --from=builder /usr/local/bin/same-telegram /usr/local/bin/same-telegram

RUN mkdir -p /data && chown -R samebot:samebot /data

ENV SAME_HOME=/data
ENV VAULT_PATH=/data/vault

USER samebot

ENTRYPOINT ["same-telegram", "serve", "--fg"]
