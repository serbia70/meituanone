package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"meituanone/internal/config"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	conn.SetMaxOpenConns(1)
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set wal mode: %w", err)
	}

	if err := ApplyStorageProfile(conn, "balanced"); err != nil {
		return nil, err
	}

	return conn, nil
}

func ApplyStorageProfile(conn *sql.DB, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "low_write"
	}

	var pragmas []string
	switch profile {
	case "low_write":
		pragmas = []string{
			"PRAGMA synchronous=NORMAL",
			"PRAGMA temp_store=MEMORY",
			"PRAGMA wal_autocheckpoint=2000",
			"PRAGMA journal_size_limit=67108864",
			"PRAGMA cache_size=-8000",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys=ON",
		}
	case "balanced":
		pragmas = []string{
			"PRAGMA synchronous=NORMAL",
			"PRAGMA temp_store=DEFAULT",
			"PRAGMA wal_autocheckpoint=1000",
			"PRAGMA journal_size_limit=33554432",
			"PRAGMA cache_size=-4000",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys=ON",
		}
	default:
		return fmt.Errorf("unknown storage profile: %s", profile)
	}

	for _, stmt := range pragmas {
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("apply %q: %w", stmt, err)
		}
	}

	return nil
}

func MigrateAndSeed(conn *sql.DB, cfg config.Config) error {
	schema := `
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
`

	if _, err := conn.Exec(schema); err != nil {
		return fmt.Errorf("run schema: %w", err)
	}

	if err := seedAdmin(conn, cfg); err != nil {
		return err
	}

	if err := seedDemoMenu(conn); err != nil {
		return err
	}

	return nil
}

func seedAdmin(conn *sql.DB, cfg config.Config) error {
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM admins").Scan(&count); err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}

	_, err = conn.Exec(
		"INSERT INTO admins (username, password_hash, created_at) VALUES (?, ?, ?)",
		cfg.AdminUser,
		string(hash),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert default admin: %w", err)
	}

	return nil
}

func seedDemoMenu(conn *sql.DB) error {
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		return fmt.Errorf("count categories: %w", err)
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer tx.Rollback()

	catStmt, err := tx.Prepare("INSERT INTO categories (name, sort_order, created_at) VALUES (?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare category insert: %w", err)
	}
	defer catStmt.Close()

	prodStmt, err := tx.Prepare("INSERT INTO products (category_id, name, price, description, sort_order, created_at) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fmt.Errorf("prepare product insert: %w", err)
	}
	defer prodStmt.Close()

	res, err := catStmt.Exec("热销推荐", 1, now)
	if err != nil {
		return fmt.Errorf("insert category 1: %w", err)
	}
	cat1, _ := res.LastInsertId()

	res, err = catStmt.Exec("饮品", 2, now)
	if err != nil {
		return fmt.Errorf("insert category 2: %w", err)
	}
	cat2, _ := res.LastInsertId()

	products := []struct {
		CatID       int64
		Name        string
		Price       int
		Description string
		Sort        int
	}{
		{CatID: cat1, Name: "牛肉汉堡", Price: 850, Description: "经典牛肉饼+生菜+芝士", Sort: 1},
		{CatID: cat1, Name: "薯条", Price: 350, Description: "外脆里软", Sort: 2},
		{CatID: cat1, Name: "鸡翅", Price: 550, Description: "香辣口味", Sort: 3},
		{CatID: cat2, Name: "可乐", Price: 250, Description: "330ml", Sort: 1},
		{CatID: cat2, Name: "柠檬茶", Price: 300, Description: "清爽解腻", Sort: 2},
	}

	for _, p := range products {
		if _, err := prodStmt.Exec(p.CatID, p.Name, p.Price, p.Description, p.Sort, now); err != nil {
			return fmt.Errorf("insert demo product: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}

	return nil
}
