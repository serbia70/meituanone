#!/usr/bin/env python3
import argparse
import json
import sqlite3
import sys
from datetime import datetime, timezone


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
        if not isinstance(item, dict):
            continue
        result.append(item)
    return result


def find_legacy_ids(src_conn, slug):
    src = src_conn.cursor()
    shop_row = src.execute("SELECT id FROM shops WHERE slug = ?", (slug,)).fetchone()
    restaurant_row = src.execute("SELECT id FROM restaurants WHERE slug = ?", (slug,)).fetchone()
    shop_id = shop_row[0] if shop_row else None
    restaurant_id = restaurant_row[0] if restaurant_row else None
    return shop_id, restaurant_id


def build_filter(shop_id, restaurant_id):
    clauses = []
    params = []
    if shop_id is not None:
        clauses.append("shop_id = ?")
        params.append(shop_id)
    if restaurant_id is not None:
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
    src_conn = sqlite3.connect(source)
    src_conn.row_factory = sqlite3.Row
    dst_conn = sqlite3.connect(target)
    dst_conn.row_factory = sqlite3.Row

    try:
        src = src_conn.cursor()
        dst = dst_conn.cursor()

        shop_id, restaurant_id = find_legacy_ids(src_conn, slug)
        if shop_id is None and restaurant_id is None:
            raise RuntimeError(f"slug '{slug}' not found in legacy shops/restaurants")

        category_filter, category_params = build_filter(shop_id, restaurant_id)
        product_filter, product_params = build_filter(shop_id, restaurant_id)
        order_filter, order_params = build_filter(shop_id, restaurant_id)

        src_cat_count = src.execute(
            f"SELECT COUNT(*) FROM categories WHERE {category_filter}", category_params
        ).fetchone()[0]
        src_prod_count = src.execute(
            f"SELECT COUNT(*) FROM products WHERE {product_filter}", product_params
        ).fetchone()[0]
        src_order_count = src.execute(
            f"SELECT COUNT(*) FROM orders WHERE {order_filter} AND COALESCE(is_deleted,0)=0", order_params
        ).fetchone()[0]

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

        cat_rows = src.execute(
            f"SELECT id, name, COALESCE(sort_order,0) AS sort_order, COALESCE(updated_at,'') AS updated_at "
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

        prod_rows = src.execute(
            f"SELECT id, category_id, name, COALESCE(price,0) AS price, COALESCE(img,'') AS img, "
            f"COALESCE(sub_name,'') AS sub_name, COALESCE(sort_order,0) AS sort_order, COALESCE(is_available,1) AS is_available, "
            f"COALESCE(updated_at,'') AS updated_at "
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
            order_rows = src.execute(
                f"SELECT id, order_no, COALESCE(total_amount,0) AS total_amount, COALESCE(order_type,'dine_in') AS order_type, "
                f"COALESCE(table_info,'') AS table_info, COALESCE(items_json,'') AS items_json, COALESCE(user_phone,'') AS user_phone, "
                f"COALESCE(status,'pending') AS status, COALESCE(created_at,'') AS created_at, COALESCE(updated_at,'') AS updated_at, "
                f"COALESCE(print_status,'pending') AS print_status, COALESCE(delivery_info,'') AS delivery_info, COALESCE(remarks_json,'') AS remarks_json "
                f"FROM orders WHERE {order_filter} AND COALESCE(is_deleted,0)=0 ORDER BY id ASC",
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

        if dry_run:
            dst_conn.rollback()
            print("[INFO] dry-run complete, rolled back changes")
            return

        dst_conn.commit()
        print(
            f"[OK] imported categories={len(cat_map)}, products={len(prod_map)}, "
            f"orders={imported_orders}, order_items={imported_items}"
        )

    finally:
        src_conn.close()
        dst_conn.close()


def main():
    parser = argparse.ArgumentParser(description="Import menu/orders from legacy meituanGo sqlite DB into meituanone DB")
    parser.add_argument("--source", required=True, help="path to legacy meituan.db")
    parser.add_argument("--target", required=True, help="path to meituanone shop.db")
    parser.add_argument("--slug", required=True, help="legacy shop slug, e.g. 05")
    parser.add_argument("--replace-menu", action="store_true", help="replace target categories/products")
    parser.add_argument("--replace-orders", action="store_true", help="replace target orders/order_items")
    parser.add_argument("--dry-run", action="store_true", help="run import and rollback")
    args = parser.parse_args()

    if not args.replace_menu and not args.replace_orders:
        print("[ERROR] choose at least one of --replace-menu or --replace-orders", file=sys.stderr)
        sys.exit(2)

    try:
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
