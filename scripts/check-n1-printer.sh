#!/usr/bin/env sh
set -eu

TEST_PRINT="${1:-}"

echo "== N1 Printer Diagnostics =="
echo "[1/6] System"
uname -a || true

echo "[2/6] USB device nodes"
if [ -d /dev/usb ]; then
  ls -la /dev/usb || true
else
  echo "[WARN] /dev/usb not found"
fi

echo "[3/6] lp device nodes"
ls -la /dev/lp* /dev/usb/lp* 2>/dev/null || echo "[WARN] no lp device found"

echo "[4/6] lsusb"
if command -v lsusb >/dev/null 2>&1; then
  lsusb || true
else
  echo "[WARN] lsusb not installed. Install with: apt install -y usbutils"
fi

echo "[5/6] Printer target"
DEVICE="${PRINTER_DEVICE:-/dev/usb/lp0}"
echo "PRINTER_DEVICE=${DEVICE}"
if [ -e "$DEVICE" ]; then
  ls -l "$DEVICE"
else
  echo "[WARN] printer device not found: $DEVICE"
fi

echo "[6/6] Docker mount check"
if command -v docker >/dev/null 2>&1; then
  docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}' || true
  echo "Tip: container should mount /dev/usb:/dev/usb"
else
  echo "[WARN] docker not installed"
fi

if [ "$TEST_PRINT" = "--print-test" ]; then
  echo "\n[TEST] Sending raw text to printer device"
  if [ ! -w "$DEVICE" ]; then
    echo "[ERROR] No write permission for $DEVICE"
    exit 1
  fi

  printf '\033@\nN1 Printer Test\nTime: %s\n\n\n\035V\101\020' "$(date '+%F %T')" > "$DEVICE"
  echo "[OK] Test bytes sent"
fi

echo "Done."
