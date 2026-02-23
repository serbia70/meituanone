#!/usr/bin/env sh
set -eu

if [ "$#" -lt 3 ]; then
  echo "Usage: $0 <shop_code> <store_name> <shop_port> [public_domain]"
  echo "Example: $0 shop01 \"Store 01\" 3001 shop01.example.com"
  exit 1
fi

SHOP_CODE="$1"
STORE_NAME="$2"
SHOP_PORT="$3"
PUBLIC_DOMAIN="${4:-${SHOP_CODE}.example.com}"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE_DIR="$ROOT_DIR/deployments/shop-template"
TARGET_DIR="$ROOT_DIR/deployments/shops/$SHOP_CODE"

if [ -d "$TARGET_DIR" ]; then
  echo "Error: target already exists: $TARGET_DIR"
  exit 1
fi

esc() {
  printf '%s' "$1" | sed -e 's/[\/&]/\\&/g'
}

SHOP_CODE_ESC="$(esc "$SHOP_CODE")"
STORE_NAME_ESC="$(esc "$STORE_NAME")"
SHOP_PORT_ESC="$(esc "$SHOP_PORT")"
PUBLIC_DOMAIN_ESC="$(esc "$PUBLIC_DOMAIN")"

mkdir -p "$TARGET_DIR/data"

cp "$TEMPLATE_DIR/docker-compose.yml" "$TARGET_DIR/docker-compose.yml"
cp "$TEMPLATE_DIR/docker-compose.tunnel.yml" "$TARGET_DIR/docker-compose.tunnel.yml"

sed \
  -e "s/__SHOP_CODE__/$SHOP_CODE_ESC/g" \
  -e "s/__STORE_NAME__/$STORE_NAME_ESC/g" \
  -e "s/__SHOP_PORT__/$SHOP_PORT_ESC/g" \
  -e "s/__PUBLIC_DOMAIN__/$PUBLIC_DOMAIN_ESC/g" \
  "$TEMPLATE_DIR/.env.template" > "$TARGET_DIR/.env"

echo "Created: $TARGET_DIR"
echo "Next steps:"
echo "1) Edit $TARGET_DIR/.env (ADMIN_PASSWORD, JWT_SECRET, CF_TUNNEL_TOKEN)"
echo "2) cd $TARGET_DIR"
echo "3) docker compose up -d"
echo "4) Optional tunnel: docker compose -f docker-compose.yml -f docker-compose.tunnel.yml up -d"
