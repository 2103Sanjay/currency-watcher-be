#!/usr/bin/env bash
#
# Deploys a Currency Watcher image on the EC2 host. It runs ON the instance,
# as root, via AWS SSM Run Command, sent by the GitHub Actions deploy job.
#
# Required environment:
#   IMAGE       full image reference, e.g. 123.dkr.ecr.ap-southeast-1.amazonaws.com/currency-watcher:<sha>
#   AWS_REGION  region of the ECR registry
#
# Configuration written by Terraform when the instance is created:
#   /etc/currency-watcher/host.env  HOST_PORT and CONTAINER_PORT (defaults 80, 8080)
#   /etc/currency-watcher/app.env   runtime settings for the API (ALLOWED_ORIGINS, CACHE_TTL, ...)
#
# If the new container fails its health check, the previous one is restored
# and the script exits non-zero so the pipeline fails.

set -euo pipefail

: "${IMAGE:?IMAGE is required}"
: "${AWS_REGION:?AWS_REGION is required}"

HOST_CONFIG=/etc/currency-watcher/host.env
if [[ -f "$HOST_CONFIG" ]]; then
  # shellcheck source=/dev/null
  source "$HOST_CONFIG"
fi

CONTAINER=currency-watcher
CONTAINER_PORT="${CONTAINER_PORT:-8080}"
HOST_PORT="${HOST_PORT:-80}"
ENV_FILE=/etc/currency-watcher/app.env
HEALTH_URL="http://localhost:${HOST_PORT}/api/health"

log() { echo "[deploy] $*"; }

start_container() {
  local image="$1"
  local env_args=()
  if [[ -f "$ENV_FILE" ]]; then
    env_args=(--env-file "$ENV_FILE")
  fi
  # -e PORT comes after --env-file so the port mapping can't be broken by it.
  docker run -d \
    --name "$CONTAINER" \
    --restart unless-stopped \
    --publish "${HOST_PORT}:${CONTAINER_PORT}" \
    --log-opt max-size=10m --log-opt max-file=3 \
    "${env_args[@]}" \
    -e PORT="$CONTAINER_PORT" \
    "$image" >/dev/null
}

wait_healthy() {
  for _ in $(seq 1 30); do
    if curl -fsS --max-time 2 "$HEALTH_URL"; then
      echo
      return 0
    fi
    sleep 1
  done
  return 1
}

log "logging in to ${IMAGE%%/*}"
aws ecr get-login-password --region "$AWS_REGION" |
  docker login --username AWS --password-stdin "${IMAGE%%/*}" >/dev/null

log "pulling $IMAGE"
docker pull --quiet "$IMAGE"

previous="$(docker inspect --format '{{.Config.Image}}' "$CONTAINER" 2>/dev/null || true)"
log "currently running: ${previous:-<nothing>}"

# Single instance, so there is a short gap (a few seconds) while containers swap.
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true

log "starting $IMAGE"
start_container "$IMAGE"

if wait_healthy; then
  log "healthy; removing unused images"
  docker image prune -af >/dev/null
  log "deployed $IMAGE"
  exit 0
fi

log "new container failed its health check; recent logs:"
docker logs --tail 50 "$CONTAINER" 2>&1 || true
docker rm -f "$CONTAINER" >/dev/null 2>&1 || true

if [[ -n "$previous" ]]; then
  log "rolling back to $previous"
  start_container "$previous"
  if wait_healthy; then
    log "rollback succeeded"
  else
    log "rollback ALSO failed; manual intervention required"
  fi
fi
exit 1
