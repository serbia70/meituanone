# Cloudflare Tunnel (No VPS)

Use this when you want public HTTPS access to your N1 service without opening router ports.

## 1) Create tunnel in Cloudflare dashboard

- Go to Zero Trust -> Networks -> Tunnels
- Create a tunnel
- Add public hostname (example: `shop01.your-domain.com`)
- Service URL: `http://app:3000`

Cloudflare gives you a token.

## 2) Set token in local `.env`

```bash
CF_TUNNEL_TOKEN=xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

## 3) Start app + tunnel

```bash
docker compose up -d
docker compose -f docker-compose.yml -f docker-compose.tunnel.yml up -d
```

## 4) Verify

```bash
docker logs -f meituanone-tunnel
```

If tunnel is healthy, open your public hostname.
