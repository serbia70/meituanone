# N1 First Go-Live Checklist

Use this checklist when bringing one new store online on an N1 box.

## 0) Prerequisites

- Debian 12 installed and reachable over SSH
- Docker installed and running
- Thermal printer connected (USB or LAN)
- Domain name ready (optional if testing LAN first)
- Cloudflare account ready if using tunnel

## 1) Prepare deployment files

- Copy `.env.n1.production.example` to `.env`
- Set `STORE_NAME`
- Set strong `ADMIN_PASSWORD`
- Set strong `JWT_SECRET` (32+ chars)
- Set printing config (`PRINTER_MODE`, `PRINTER_DEVICE` or `PRINTER_TCP`)

## 2) Deploy app container

```bash
docker compose up -d
```

Expected:

- Container `meituanone-shop` is running
- `http://<N1-IP>:3000/health` returns `{"ok":true,...}`

## 3) Validate printer path and permissions

```bash
chmod +x scripts/check-n1-printer.sh
./scripts/check-n1-printer.sh
```

Optional raw test print:

```bash
PRINTER_DEVICE=/dev/usb/lp0 ./scripts/check-n1-printer.sh --print-test
```

## 4) Functional checks

- Open customer page `/`
- Create one test order
- Confirm printer outputs ticket
- Open admin page `/admin`
- Confirm new order appears in admin list
- Confirm status update works and reprint works

## 5) Optional public access (no VPS)

- Put `CF_TUNNEL_TOKEN` in `.env`
- Start tunnel sidecar:

```bash
docker compose -f docker-compose.yml -f docker-compose.tunnel.yml up -d
```

- Confirm `meituanone-tunnel` logs show healthy connection
- Open public hostname and place one external test order

## 6) Backup and recovery check

- Stop app and copy `data/shop.db` to backup location
- Start app again and verify service healthy
- Keep at least daily backup policy for each store

## 7) Final handover

- Remove default credentials
- Save store deployment record (domain, admin account, printer mode, image tag)
- Capture a final test order screenshot and printed receipt photo
