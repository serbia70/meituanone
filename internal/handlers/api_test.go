package handlers

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"meituanone/internal/config"
	"meituanone/internal/db"
	"meituanone/internal/printer"

	"github.com/gin-gonic/gin"
)

func TestListOrders_DoesNotHangWithSingleDBConn(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dbPath := filepath.Join(t.TempDir(), "shop.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	cfg := config.Config{AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test-secret", TokenTTL: 24 * time.Hour}
	if err := db.MigrateAndSeed(conn, cfg); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	_, err = conn.Exec(`
INSERT INTO orders (order_no, customer_name, customer_phone, order_type, address, note, total_amount, status, print_status, created_at, updated_at)
VALUES ('O-test-1', 'Alice', '123', 'dine_in', '', '', 1000, 'pending', 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
`)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}

	_, err = conn.Exec(`
INSERT INTO order_items (order_id, product_id, product_name, unit_price, qty, subtotal, created_at)
VALUES (1, 1, 'Demo', 500, 2, 1000, CURRENT_TIMESTAMP)
`)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}

	app := New(conn, cfg, printer.New(printer.Config{Mode: "stdout"}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/admin/orders", nil)

	done := make(chan struct{})
	go func() {
		app.listOrders(c)
		close(done)
	}()

	select {
	case <-done:
		if w.Code != 200 {
			t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("listOrders handler hung (possible sqlite single-conn deadlock)")
	}
}
