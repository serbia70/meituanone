# MeituanOne (Standalone)

Single-shop ordering system for N1 boxes.

## Features

- Customer ordering page (`/`)
- Admin order console (`/admin`)
- SQLite local database
- Thermal receipt printing (ESC/POS)
- Docker deployment on Debian 12 (N1)
- GitHub Actions build to GHCR

## Quick Start (Local)

1. Copy env:

```bash
cp .env.example .env
```

2. Run server:

```bash
go mod tidy
go run ./cmd/server
```

3. Open:

- Customer page: `http://localhost:3000/`
- Admin page: `http://localhost:3000/admin`

## Printing

`PRINTER_MODE` options:

- `stdout`: debug mode
- `file`: write to a device file (e.g. `/dev/usb/lp0`)
- `tcp`: network printer (e.g. `192.168.1.50:9100`)

## Low-Write Mode (N1 eMMC protection)

Defaults are optimized for lower disk wear:

- `STORAGE_PROFILE=low_write`
- `ACCESS_LOG=false`
- `GIN_MODE=release`
- SQLite WAL with larger checkpoint interval
- Docker log rotation (`5m x 2`)
- Container `/tmp` on tmpfs

If you need more debug logs temporarily, set:

```bash
ACCESS_LOG=true
GIN_MODE=debug
STORAGE_PROFILE=balanced
```

After troubleshooting, switch back to low-write settings.

## N1 Deployment

```bash
chmod +x scripts/deploy-n1.sh
./scripts/deploy-n1.sh ghcr.io/serbia70/meituanone:latest
```

### First go-live checklist

Use: `docs/ops/n1-go-live-checklist.md`.

### N1 Troubleshooting Checklist (USB Thermal Printer)

Run diagnostics:

```bash
chmod +x scripts/check-n1-printer.sh
./scripts/check-n1-printer.sh
```

Optional test print (raw ESC/POS):

```bash
PRINTER_DEVICE=/dev/usb/lp0 ./scripts/check-n1-printer.sh --print-test
```

Common checks:

- Printer device exists (`/dev/usb/lp0`)
- Docker container mounts `/dev/usb:/dev/usb`
- `.env` has `PRINTER_MODE=file`
- `.env` has correct `PRINTER_DEVICE`

### Recommended N1 .env

Use the production template:

```bash
cp .env.n1.production.example .env
```

Then edit at least:

- `STORE_NAME`
- `ADMIN_PASSWORD`
- `JWT_SECRET`
- `PRINTER_MODE` and `PRINTER_DEVICE`

For USB thermal printers on Debian 12, default is:

- `PRINTER_MODE=file`
- `PRINTER_DEVICE=/dev/usb/lp0`

## Public Access Without VPS (Cloudflare Tunnel)

1) Put tunnel token in `.env`:

```bash
CF_TUNNEL_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

2) Start tunnel sidecar:

```bash
docker compose -f docker-compose.yml -f docker-compose.tunnel.yml up -d
```

3) Check tunnel logs:

```bash
docker logs -f meituanone-tunnel
```

More details: `cloudflared/README.md`.

## Multi-store batch setup

Use templates in `deployments/shop-template` and generate a store folder:

```bash
chmod +x scripts/create-shop.sh
scripts/create-shop.sh shop01 "Store 01" 3001 shop01.example.com
```

Generate many stores in one command:

```bash
chmod +x scripts/create-shops-batch.sh
scripts/create-shops-batch.sh shop 1 20 3001 example.com Store
```

Generated folder:

```text
deployments/shops/shop01/
  .env
  docker-compose.yml
  docker-compose.tunnel.yml
  data/
```

Then deploy from that folder:

```bash
cd deployments/shops/shop01
docker compose up -d
```

## GitHub Build

Workflow file: `.github/workflows/docker.yml`

When pushing to `main`, GitHub Actions builds and pushes:

- `ghcr.io/serbia70/meituanone:latest`
- `ghcr.io/serbia70/meituanone:<commit_sha>`

## Environment Variables

See `.env.example`.
