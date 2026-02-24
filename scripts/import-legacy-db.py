#!/usr/bin/env python3
import argparse
import json
import os
import sqlite3
import sys
from datetime import datetime, timezone


TARGET_SCHEMA = """
CREATE TABLE IF NOT EXISTS admins (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  email TEXT,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS categories (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  category_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  price INTEGER NOT NULL,
  image TEXT,
  description TEXT,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (category_id) REFERENCES categories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS orders (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_no TEXT NOT NULL UNIQUE,
  customer_name TEXT,
  customer_phone TEXT,
  order_type TEXT NOT NULL,
  address TEXT,
  note TEXT,
  total_amount INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  print_status TEXT NOT NULL DEFAULT 'pending',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS order_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  order_id INTEGER NOT NULL,
  product_id INTEGER NOT NULL,
  product_name TEXT NOT NULL,
  unit_price INTEGER NOT NULL,
  qty INTEGER NOT NULL,
  subtotal INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_products_category ON products(category_id);
"""


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S")


def safe_int(value, default=0):
    try:
        if value is None:
            return default
        if isinstance(value, bool):
            return int(value)
        if isinstance(value, (int, float)):
            return int(value)
        text = str(value).strip()
        if text == "":
            return default
        return int(float(text))
    except Exception:
        return default


def first_non_empty(dct, *keys):
    for key in keys:
        value = dct.get(key)
        if value is None:
            continue
        text = str(value).strip()
        if text != "":
            return text
    return ""


def parse_items(items_json):
    if not items_json:
        return []
    try:
        data = json.loads(items_json)
    except Exception:
        return []
    if not isinstance(data, list):
        return []

    result = []
    for item in data:
        if isinstance(item, dict):
            result.append(item)
    return result


def ensure_target_schema(conn):
    conn.executescript(TARGET_SCHEMA)
    conn.commit()


def table_exists(conn, table_name):
    cur = conn.cursor()
    row = cur.execute(
        "SELECT 1 FROM sqlite_master WHERE type='table' AND name=? LIMIT 1", (table_name,)
    ).fetchone()
    return bool(row)


def table_columns(conn, table_name):
    if not table_exists(conn, table_name):
        return set()
    cur = conn.cursor()
    rows = cur.execute(f"PRAGMA table_info({table_name})").fetchall()
    return {r[1] for r in rows}


def select_col_value(columns, name, fallback_sql):
    if name in columns:
        return name
    return fallback_sql


def select_col_as(columns, name, fallback_sql):
    return f"{select_col_value(columns, name, fallback_sql)} AS {name}"


def find_legacy_ids(src_conn, slug):
    src = src_conn.cursor()

    shop_id = None
    if table_exists(src_conn, "shops"):
        shop_row = src.execute("SELECT id FROM shops WHERE slug = ?", (slug,)).fetchone()
        shop_id = shop_row[0] if shop_row else None

    restaurant_id = None
    if table_exists(src_conn, "restaurants"):
        restaurant_row = src.execute("SELECT id FROM restaurants WHERE slug = ?", (slug,)).fetchone()
        restaurant_id = restaurant_row[0] if restaurant_row else None

    return shop_id, restaurant_id


def list_legacy_slugs(src_conn):
    cur = src_conn.cursor()
    slugs = set()

    if table_exists(src_conn, "shops"):
        rows = cur.execute("SELECT slug FROM shops WHERE TRIM(COALESCE(slug, '')) != ''").fetchall()
        slugs.update(row[0] for row in rows if row[0])

    if table_exists(src_conn, "restaurants"):
        rows = cur.execute("SELECT slug FROM restaurants WHERE TRIM(COALESCE(slug, '')) != ''").fetchall()
        slugs.update(row[0] for row in rows if row[0])

    return sorted(slugs)


def build_filter_for_table(columns, shop_id, restaurant_id):
    clauses = []
    params = []
    if shop_id is not None and "shop_id" in columns:
        clauses.append("shop_id = ?")
        params.append(shop_id)
    if restaurant_id is not None and "restaurant_id" in columns:
        clauses.append("restaurant_id = ?")
        params.append(restaurant_id)
    if not clauses:
        return "1=0", []
    return "(" + " OR ".join(clauses) + ")", params


def ensure_unique_order_no(dst_conn, order_no):
    base = (order_no or "").strip() or f"legacy-{int(datetime.now(timezone.utc).timestamp())}"
    candidate = base
    idx = 1
    cur = dst_conn.cursor()
    while True:
        row = cur.execute("SELECT 1 FROM orders WHERE order_no = ? LIMIT 1", (candidate,)).fetchone()
        if not row:
            return candidate
        idx += 1
        candidate = f"{base}-m{idx}"


def import_legacy(source, target, slug, replace_menu, replace_orders, dry_run):
    os.makedirs(os.path.dirname(target), exist_ok=True)

    src_conn = sqlite3.connect(source)
    src_conn.row_factory = sqlite3.Row
    dst_conn = sqlite3.connect(target)
    dst_conn.row_factory = sqlite3.Row

    try:
        ensure_target_schema(dst_conn)

        src = src_conn.cursor()
        dst = dst_conn.cursor()

        if not table_exists(src_conn, "categories"):
            raise RuntimeError("legacy database missing table: categories")
        if not table_exists(src_conn, "products"):
            raise RuntimeError("legacy database missing table: products")
        if not table_exists(src_conn, "orders"):
            raise RuntimeError("legacy database missing table: orders")

        category_cols = table_columns(src_conn, "categories")
        product_cols = table_columns(src_conn, "products")
        order_cols = table_columns(src_conn, "orders")

        shop_id, restaurant_id = find_legacy_ids(src_conn, slug)
        if shop_id is None and restaurant_id is None:
            raise RuntimeError(f"slug '{slug}' not found in legacy shops/restaurants")

        category_filter, category_params = build_filter_for_table(category_cols, shop_id, restaurant_id)
        product_filter, product_params = build_filter_for_table(product_cols, shop_id, restaurant_id)
        order_filter, order_params = build_filter_for_table(order_cols, shop_id, restaurant_id)

        order_not_deleted = "COALESCE(is_deleted,0)=0" if "is_deleted" in order_cols else "1=1"

        src_cat_count = src.execute(
            f"SELECT COUNT(*) FROM categories WHERE {category_filter}", category_params
        ).fetchone()[0]
        src_prod_count = src.execute(
            f"SELECT COUNT(*) FROM products WHERE {product_filter}", product_params
        ).fetchone()[0]
        src_order_count = src.execute(f"SELECT COUNT(*) FROM orders WHERE {order_filter} AND {order_not_deleted}", order_params).fetchone()[0]

        print(f"[INFO] legacy slug={slug} shop_id={shop_id} restaurant_id={restaurant_id}")
        print(f"[INFO] source categories={src_cat_count}, products={src_prod_count}, orders={src_order_count}")

        dst.execute("BEGIN")

        if replace_orders:
            dst.execute("DELETE FROM order_items")
            dst.execute("DELETE FROM orders")

        if replace_menu:
            dst.execute("DELETE FROM products")
            dst.execute("DELETE FROM categories")

        cat_map = {}
        prod_map = {}

        if replace_menu:
            cat_updated_expr = select_col_as(category_cols, "updated_at", "''")
            cat_rows = src.execute(
                f"SELECT id, name, COALESCE(sort_order,0) AS sort_order, {cat_updated_expr} "
                f"FROM categories WHERE {category_filter} ORDER BY sort_order ASC, id ASC",
                category_params,
            ).fetchall()

            for row in cat_rows:
                created = row["updated_at"] if row["updated_at"] else now_iso()
                dst.execute(
                    "INSERT INTO categories (name, sort_order, is_active, created_at) VALUES (?, ?, 1, ?)",
                    (row["name"], safe_int(row["sort_order"], 0), created),
                )
                cat_map[row["id"]] = dst.lastrowid

            prod_img_expr = select_col_value(product_cols, "img", "''")
            prod_sub_name_expr = select_col_value(product_cols, "sub_name", "''")
            prod_updated_expr = select_col_value(product_cols, "updated_at", "''")
            prod_is_available_expr = select_col_value(product_cols, "is_available", "1")
            prod_rows = src.execute(
                f"SELECT id, category_id, name, COALESCE(price,0) AS price, COALESCE({prod_img_expr},'') AS img, "
                f"COALESCE({prod_sub_name_expr},'') AS sub_name, COALESCE(sort_order,0) AS sort_order, COALESCE({prod_is_available_expr},1) AS is_available, "
                f"COALESCE({prod_updated_expr},'') AS updated_at "
                f"FROM products WHERE {product_filter} ORDER BY sort_order ASC, id ASC",
                product_params,
            ).fetchall()

            for row in prod_rows:
                new_cat_id = cat_map.get(row["category_id"])
                if not new_cat_id:
                    continue
                created = row["updated_at"] if row["updated_at"] else now_iso()
                dst.execute(
                    "INSERT INTO products (category_id, name, price, image, description, sort_order, is_active, created_at) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
                    (
                        new_cat_id,
                        row["name"],
                        safe_int(row["price"], 0),
                        row["img"],
                        row["sub_name"],
                        safe_int(row["sort_order"], 0),
                        1 if safe_int(row["is_available"], 1) != 0 else 0,
                        created,
                    ),
                )
                prod_map[row["id"]] = dst.lastrowid

        imported_orders = 0
        imported_items = 0

        if replace_orders:
            order_table_info_expr = select_col_value(order_cols, "table_info", "''")
            order_items_json_expr = select_col_value(order_cols, "items_json", "''")
            order_user_phone_expr = select_col_value(order_cols, "user_phone", "''")
            order_updated_expr = select_col_value(order_cols, "updated_at", "''")
            order_print_status_expr = select_col_value(order_cols, "print_status", "'pending'")
            order_delivery_info_expr = select_col_value(order_cols, "delivery_info", "''")
            order_remarks_expr = select_col_value(order_cols, "remarks_json", "''")
            order_rows = src.execute(
                f"SELECT id, order_no, COALESCE(total_amount,0) AS total_amount, COALESCE(order_type,'dine_in') AS order_type, "
                f"COALESCE({order_table_info_expr},'') AS table_info, COALESCE({order_items_json_expr},'') AS items_json, COALESCE({order_user_phone_expr},'') AS user_phone, "
                f"COALESCE(status,'pending') AS status, COALESCE(created_at,'') AS created_at, COALESCE({order_updated_expr},'') AS updated_at, "
                f"COALESCE({order_print_status_expr},'pending') AS print_status, COALESCE({order_delivery_info_expr},'') AS delivery_info, COALESCE({order_remarks_expr},'') AS remarks_json "
                f"FROM orders WHERE {order_filter} AND {order_not_deleted} ORDER BY id ASC",
                order_params,
            ).fetchall()

            for row in order_rows:
                created_at = row["created_at"] if row["created_at"] else now_iso()
                updated_at = row["updated_at"] if row["updated_at"] else created_at
                order_no = ensure_unique_order_no(dst_conn, row["order_no"])

                customer_name = ""
                customer_phone = row["user_phone"]
                address = ""
                note = row["table_info"]

                try:
                    delivery = json.loads(row["delivery_info"]) if row["delivery_info"] else {}
                    if isinstance(delivery, dict):
                        customer_name = first_non_empty(delivery, "name", "customer_name", "receiver")
                        if not customer_phone:
                            customer_phone = first_non_empty(delivery, "phone", "mobile", "customer_phone")
                        address = first_non_empty(delivery, "address", "detail", "delivery_address")
                except Exception:
                    pass

                if row["remarks_json"]:
                    note = row["remarks_json"]

                dst.execute(
                    "INSERT INTO orders (order_no, customer_name, customer_phone, order_type, address, note, total_amount, status, print_status, created_at, updated_at) "
                    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
                    (
                        order_no,
                        customer_name,
                        customer_phone,
                        row["order_type"] or "dine_in",
                        address,
                        note,
                        safe_int(row["total_amount"], 0),
                        row["status"] or "pending",
                        row["print_status"] or "pending",
                        created_at,
                        updated_at,
                    ),
                )
                new_order_id = dst.lastrowid
                imported_orders += 1

                items = parse_items(row["items_json"])
                for item in items:
                    old_pid = safe_int(item.get("product_id", item.get("id")), 0)
                    new_pid = prod_map.get(old_pid, 0)
                    qty = max(safe_int(item.get("qty", item.get("quantity", item.get("count"))), 1), 1)
                    unit_price = safe_int(item.get("price", item.get("unit_price", item.get("original_price"))), 0)
                    subtotal = safe_int(item.get("subtotal"), qty * unit_price)
                    pname = first_non_empty(item, "name", "title", "product_name")

                    dst.execute(
                        "INSERT INTO order_items (order_id, product_id, product_name, unit_price, qty, subtotal, created_at) "
                        "VALUES (?, ?, ?, ?, ?, ?, ?)",
                        (new_order_id, new_pid, pname or "Unknown", unit_price, qty, subtotal, created_at),
                    )
                    imported_items += 1

        summary = {
            "slug": slug,
            "categories": len(cat_map),
            "products": len(prod_map),
            "orders": imported_orders,
            "order_items": imported_items,
            "source_orders": src_order_count,
            "target": target,
        }

        if dry_run:
            dst_conn.rollback()
            print("[INFO] dry-run complete, rolled back changes")
            return summary

        dst_conn.commit()
        print(
            f"[OK] imported categories={len(cat_map)}, products={len(prod_map)}, "
            f"orders={imported_orders}, order_items={imported_items}"
        )
        return summary

    finally:
        src_conn.close()
        dst_conn.close()


def import_all_shops(source, target_dir, replace_menu, replace_orders, dry_run, only_slugs):
    src_conn = sqlite3.connect(source)
    try:
        slugs = list_legacy_slugs(src_conn)
    finally:
        src_conn.close()

    if only_slugs:
        wanted = {x.strip() for x in only_slugs.split(",") if x.strip()}
        slugs = [s for s in slugs if s in wanted]

    if not slugs:
        print("[WARN] no shops matched for import")
        return []

    print(f"[INFO] importing {len(slugs)} shops into {target_dir}")

    results = []
    for slug in slugs:
        target = os.path.join(target_dir, slug, "data", "shop.db")
        print(f"\n=== [{slug}] -> {target} ===")
        result = import_legacy(
            source=source,
            target=target,
            slug=slug,
            replace_menu=replace_menu,
            replace_orders=replace_orders,
            dry_run=dry_run,
        )
        results.append(result)

    return results


def main():
    parser = argparse.ArgumentParser(description="Import legacy meituanGo sqlite data into meituanone DB")
    parser.add_argument("--source", required=True, help="path to legacy meituan.db")
    parser.add_argument("--target", help="path to single target shop.db (single-shop mode)")
    parser.add_argument("--slug", help="legacy shop slug in single-shop mode, e.g. 05")
    parser.add_argument("--all-shops", action="store_true", help="import all legacy slugs")
    parser.add_argument("--target-dir", help="base directory for all-shops mode, e.g. deployments/shops")
    parser.add_argument("--slugs", help="comma-separated subset in all-shops mode, e.g. 01,02,rtiam")
    parser.add_argument("--replace-menu", action="store_true", help="replace target categories/products")
    parser.add_argument("--replace-orders", action="store_true", help="replace target orders/order_items")
    parser.add_argument("--dry-run", action="store_true", help="run import and rollback")
    args = parser.parse_args()

    if not args.replace_menu and not args.replace_orders:
        print("[ERROR] choose at least one of --replace-menu or --replace-orders", file=sys.stderr)
        sys.exit(2)

    try:
        if args.all_shops:
            if not args.target_dir:
                print("[ERROR] --target-dir is required with --all-shops", file=sys.stderr)
                sys.exit(2)

            results = import_all_shops(
                source=args.source,
                target_dir=args.target_dir,
                replace_menu=args.replace_menu,
                replace_orders=args.replace_orders,
                dry_run=args.dry_run,
                only_slugs=args.slugs,
            )

            total_menu = sum(r["products"] for r in results)
            total_orders = sum(r["orders"] for r in results)
            print(f"\n[SUMMARY] shops={len(results)} products={total_menu} orders={total_orders}")
        else:
            if not args.target or not args.slug:
                print("[ERROR] --target and --slug are required in single-shop mode", file=sys.stderr)
                sys.exit(2)

            import_legacy(
                source=args.source,
                target=args.target,
                slug=args.slug,
                replace_menu=args.replace_menu,
                replace_orders=args.replace_orders,
                dry_run=args.dry_run,
            )

    except Exception as exc:
        print(f"[ERROR] {exc}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
