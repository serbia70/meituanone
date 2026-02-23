#!/usr/bin/env sh
set -eu

REPO_URL="${REPO_URL:-https://github.com/serbia70/meituanone.git}"
APP_DIR="${APP_DIR:-/opt/meituanone}"
IMAGE="${IMAGE:-ghcr.io/serbia70/meituanone:latest}"

log() {
  printf '%s\n' "[bootstrap] $*"
}

require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    log "Please run as root: sudo sh scripts/bootstrap-n1.sh"
    exit 1
  fi
}

install_base_tools() {
  log "Installing base tools (git/curl/ca-certificates)"
  apt-get update
  apt-get install -y ca-certificates curl git
}

install_docker_if_needed() {
  if command -v docker >/dev/null 2>&1; then
    log "Docker already installed: $(docker --version)"
    return
  fi

  log "Installing Docker"
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
}

clone_or_update_repo() {
  if [ -d "$APP_DIR/.git" ]; then
    log "Repository exists, pulling latest"
    git -C "$APP_DIR" pull --ff-only
    return
  fi

  if [ -e "$APP_DIR" ] && [ ! -d "$APP_DIR/.git" ]; then
    log "Target path exists but is not a git repo: $APP_DIR"
    exit 1
  fi

  log "Cloning repository to $APP_DIR"
  mkdir -p "$(dirname "$APP_DIR")"
  git clone "$REPO_URL" "$APP_DIR"
}

prepare_env() {
  if [ ! -f "$APP_DIR/.env" ]; then
    log "Creating .env from .env.n1.production.example"
    cp "$APP_DIR/.env.n1.production.example" "$APP_DIR/.env"
    log "Please edit $APP_DIR/.env (STORE_NAME, ADMIN_PASSWORD, JWT_SECRET, PRINTER_*)"
  else
    log ".env already exists, keep current values"
  fi
}

login_ghcr_if_configured() {
  if [ -n "${GHCR_USER:-}" ] && [ -n "${GHCR_PAT:-}" ]; then
    log "Logging into ghcr.io as $GHCR_USER"
    printf '%s' "$GHCR_PAT" | docker login ghcr.io -u "$GHCR_USER" --password-stdin
  else
    log "Skipping ghcr login (set GHCR_USER and GHCR_PAT if image is private)"
  fi
}

deploy() {
  chmod +x "$APP_DIR/scripts/deploy-n1.sh"
  log "Deploying image: $IMAGE"
  "$APP_DIR/scripts/deploy-n1.sh" "$IMAGE"
}

post_check() {
  log "Health check"
  sleep 2
  curl -fsS "http://127.0.0.1:3000/health" || {
    log "Health check failed"
    exit 1
  }
  printf '\n'
  log "Done. Open: http://<N1-IP>:3000"
}

require_root
install_base_tools
install_docker_if_needed
clone_or_update_repo
prepare_env
login_ghcr_if_configured
deploy
post_check
