#!/usr/bin/env sh
set -eu

if [ "$#" -lt 5 ]; then
  echo "Usage: $0 <prefix> <start_no> <end_no> <start_port> <domain_base> [store_name_prefix]"
  echo "Example: $0 shop 1 20 3001 example.com Store"
  exit 1
fi

PREFIX="$1"
START_NO="$2"
END_NO="$3"
START_PORT="$4"
DOMAIN_BASE="$5"
STORE_PREFIX="${6:-Store}"

case "$START_NO" in
  ''|*[!0-9]*) echo "Error: start_no must be number"; exit 1 ;;
esac
case "$END_NO" in
  ''|*[!0-9]*) echo "Error: end_no must be number"; exit 1 ;;
esac
case "$START_PORT" in
  ''|*[!0-9]*) echo "Error: start_port must be number"; exit 1 ;;
esac

if [ "$END_NO" -lt "$START_NO" ]; then
  echo "Error: end_no must be >= start_no"
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ONE_SCRIPT="$ROOT_DIR/scripts/create-shop.sh"

if [ ! -x "$ONE_SCRIPT" ]; then
  chmod +x "$ONE_SCRIPT"
fi

count=0
i="$START_NO"
while [ "$i" -le "$END_NO" ]; do
  num="$(printf "%02d" "$i")"
  code="${PREFIX}${num}"
  name="${STORE_PREFIX} ${num}"
  port="$((START_PORT + i - START_NO))"
  domain="${code}.${DOMAIN_BASE}"

  "$ONE_SCRIPT" "$code" "$name" "$port" "$domain"

  count="$((count + 1))"
  i="$((i + 1))"
done

echo "Done. Generated ${count} store folders under deployments/shops/."
