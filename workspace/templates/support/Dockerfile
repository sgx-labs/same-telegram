# --- Build SAME binary ---
FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends libsqlite3-dev && \
    rm -rf /var/lib/apt/lists/* && \
    CGO_ENABLED=1 go install github.com/sgx-labs/statelessagent/cmd/same@latest && \
    cp $(go env GOPATH)/bin/same /usr/local/bin/same

# --- Runtime ---
FROM python:3.12-slim-bookworm

# System deps for SAME (SQLite)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libsqlite3-0 \
    && rm -rf /var/lib/apt/lists/*

# SAME binary
COPY --from=builder /usr/local/bin/same /usr/local/bin/same

# Python deps
COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt

# SAME Python SDK (from parent templates directory)
COPY same_sdk/ /app/same_sdk/

# Bot code and knowledge base
COPY bot.py /app/bot.py
COPY knowledge/ /app/knowledge/

WORKDIR /app

# Initialize vault and start bot.
# The bot ingests the knowledge/ directory into SAME on startup.
CMD ["sh", "-c", "same init --path ${VAULT_PATH:-/data/vault} 2>/dev/null; exec python bot.py"]
