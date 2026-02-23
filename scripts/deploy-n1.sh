#!/usr/bin/env sh
set -eu

IMAGE="${1:-ghcr.io/serbia70/meituanone:latest}"
APP_NAME="meituanone-shop"
BASE_DIR="/opt/meituanone"

mkdir -p "$BASE_DIR/data"

if [ ! -f "$BASE_DIR/.env" ]; then
  echo "[INFO] .env not found, creating from template values"
  cat > "$BASE_DIR/.env" <<'EOF'
PORT=3000
WEB_DIR=/app/web
DB_PATH=/app/data/shop.db
STORAGE_PROFILE=low_write
ACCESS_LOG=false
GIN_MODE=release
STORE_NAME=My Store
ADMIN_USER=admin
ADMIN_PASSWORD=admin123
JWT_SECRET=change-this-secret
TOKEN_TTL_HOURS=720
AUTO_PRINT=true
PRINTER_MODE=stdout
PRINTER_DEVICE=/dev/usb/lp0
PRINTER_TCP=
CORS_ORIGIN=*
EOF
fi

docker pull "$IMAGE"

if docker ps -a --format '{{.Names}}' | grep -q "^${APP_NAME}$"; then
  docker rm -f "$APP_NAME"
fi

docker run -d \
  --name "$APP_NAME" \
  --restart unless-stopped \
  --log-driver json-file \
  --log-opt max-size=5m \
  --log-opt max-file=2 \
  -p 3000:3000 \
  --env-file "$BASE_DIR/.env" \
  -v "$BASE_DIR/data:/app/data" \
  -v /dev/usb:/dev/usb \
  --tmpfs /tmp:rw,size=32m \
  "$IMAGE"

echo "[OK] deployed: $APP_NAME"
echo "[OK] open: http://<N1-IP>:3000"
