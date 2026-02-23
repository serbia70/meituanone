# Multi-Store Deployment Layout

This repository keeps one shared codebase and many per-store deployment folders.

## Structure

```text
deployments/
  shop-template/
    .env.template
    docker-compose.yml
    docker-compose.tunnel.yml
  shops/
    shop01/
      .env
      docker-compose.yml
      docker-compose.tunnel.yml
      data/
```

## Create a new store folder

```bash
chmod +x scripts/create-shop.sh
scripts/create-shop.sh shop01 "Store 01" 3001 shop01.example.com
```

## Batch create many stores (shop01-shop20)

```bash
chmod +x scripts/create-shops-batch.sh
scripts/create-shops-batch.sh shop 1 20 3001 example.com Store
```

This generates:

- shop code: `shop01` ... `shop20`
- store name: `Store 01` ... `Store 20`
- host port: `3001` ... `3020`
- domain: `shop01.example.com` ... `shop20.example.com`

## Notes

- Each store gets its own `data/shop.db`
- Each store should use its own domain and Cloudflare tunnel token
- On one host, each store must use a unique `SHOP_PORT`
