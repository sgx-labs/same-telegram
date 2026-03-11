#!/usr/bin/env bash
#
# deploy.sh — Deploy SameVault workspace infrastructure.
#
# Prerequisites:
#   - flyctl installed (https://fly.io/docs/flyctl/install/)
#   - Authenticated: fly auth login
#   - Docker running locally
#
# Usage:
#   ./workspace/deploy.sh          # Full deploy (create app + build + deploy)
#   ./workspace/deploy.sh build    # Build image only
#   ./workspace/deploy.sh push     # Push image only
#
# Environment variables (required):
#   FLY_API_TOKEN    — Fly deploy token (fly tokens deploy)
#
# Environment variables (optional):
#   FLY_APP_NAME     — App name (default: samevault-workspaces)
#   FLY_REGION       — Primary region (default: iad)
#   FLY_ORG          — Fly org (default: personal)

set -euo pipefail

APP_NAME="${FLY_APP_NAME:-samevault-workspaces}"
REGION="${FLY_REGION:-iad}"
ORG="${FLY_ORG:-personal}"
IMAGE="registry.fly.io/${APP_NAME}:latest"

cd "$(dirname "$0")/.."

log() { echo "==> $*"; }
err() { echo "ERROR: $*" >&2; exit 1; }

# --- Preflight checks ---
command -v fly >/dev/null 2>&1 || err "flyctl not found. Install: https://fly.io/docs/flyctl/install/"
command -v docker >/dev/null 2>&1 || err "docker not found"

if [ -z "${FLY_API_TOKEN:-}" ]; then
  err "FLY_API_TOKEN not set. Generate one: fly tokens deploy -a ${APP_NAME}"
fi

cmd="${1:-deploy}"

case "$cmd" in
  build)
    log "Building workspace image..."
    docker build -f Dockerfile.workspace -t "${IMAGE}" .
    log "Build complete: ${IMAGE}"
    ;;

  push)
    log "Pushing image to Fly registry..."
    fly auth docker
    docker push "${IMAGE}"
    log "Push complete"
    ;;

  deploy)
    # Step 1: Create app if it doesn't exist
    if ! fly apps list --json 2>/dev/null | grep -q "\"${APP_NAME}\""; then
      log "Creating Fly app: ${APP_NAME}"
      fly apps create "${APP_NAME}" --org "${ORG}"
    else
      log "App ${APP_NAME} already exists"
    fi

    # Step 2: Build and push
    log "Building workspace image..."
    docker build -f Dockerfile.workspace -t "${IMAGE}" .

    log "Pushing to Fly registry..."
    fly auth docker
    docker push "${IMAGE}"

    log ""
    log "=== Deploy complete ==="
    log ""
    log "Image: ${IMAGE}"
    log "App:   ${APP_NAME}"
    log "Region: ${REGION}"
    log ""
    log "The bot will create machines on-demand when users tap /start."
    log "No machines are running until a user triggers provisioning."
    log ""
    log "Bot config needed (telegram.toml or env vars):"
    log "  [bot]"
    log "  mode = \"workspace\""
    log "  fly_api_token = \"${FLY_API_TOKEN:0:8}...\""
    log "  fly_app_name = \"${APP_NAME}\""
    log "  fly_region = \"${REGION}\""
    log "  fly_image = \"${IMAGE}\""
    log ""
    ;;

  *)
    err "Unknown command: ${cmd}. Use: build, push, or deploy"
    ;;
esac
