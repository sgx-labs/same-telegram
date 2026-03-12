#!/usr/bin/env bash
# Deploy workspace image and update the bot to use it.
#
# Usage: ./scripts/deploy-workspace.sh
#
# This script:
# 1. Deploys the workspace image (samevault-workspaces)
# 2. Extracts the deployment image tag
# 3. Updates the bot's FLY_IMAGE env var to point to the new image
# 4. Redeploys the bot (same-telegram)

set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> Deploying workspace image..."
OUTPUT=$(fly deploy --config fly.workspace.toml --dockerfile Dockerfile.workspace -a samevault-workspaces 2>&1)
echo "$OUTPUT"

# Extract the image tag from deploy output
IMAGE=$(echo "$OUTPUT" | sed -n 's/^image: \(.*\)/\1/p')
if [ -z "$IMAGE" ]; then
    echo "ERROR: Could not extract image tag from deploy output"
    exit 1
fi
echo ""
echo "==> Workspace image: $IMAGE"

# Update fly.toml with the new image tag
sed -i "s|FLY_IMAGE = \".*\"|FLY_IMAGE = \"$IMAGE\"|" fly.toml
echo "==> Updated fly.toml FLY_IMAGE to: $IMAGE"

echo ""
echo "==> Redeploying bot with new image..."
fly deploy --config fly.toml

echo ""
echo "==> Done. New workspaces will use: $IMAGE"
echo "==> Destroy existing workspaces and /start fresh to pick up the new image."
