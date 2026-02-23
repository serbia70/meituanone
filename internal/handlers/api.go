package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"meituanone/internal/config"
	"meituanone/internal/printer"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type App struct {
	DB      *sql.DB
	Cfg     config.Config
	Printer *printer.Service
	Events  *eventHub
}

type jwtClaims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func New(db *sql.DB, cfg config.Config, printerSvc *printer.Service) *App {
	return &App{DB: db, Cfg: cfg, Printer: printerSvc, Events: newEventHub()}
}

func (a *App) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", a.health)
	r.GET("/api/menu", a.getMenu)
	r.POST("/api/order", a.createOrder)
	r.GET("/api/admin/events", a.adminEvents)

	admin := r.Group("/api/admin")
	admin.POST("/login", a.adminLogin)
	admin.Use(a.adminAuth())
	{
		admin.GET("/orders", a.listOrders)
		admin.PATCH("/orders/:id/status", a.updateOrderStatus)
		admin.POST("/orders/:id/print", a.printOrder)
	}
}

type eventHub struct {
	mu      sync.RWMutex
	clients map[chan string]struct{}
}

func newEventHub() *eventHub {
	return &eventHub{clients: make(map[chan string]struct{})}
}

func (h *eventHub) Subscribe(ch chan string) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *eventHub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *eventHub) Broadcast(msg string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *eventHub) BroadcastJSON(v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.Broadcast(string(b))
}

func (a *App) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok":         true,
		"store_name": a.Cfg.StoreName,
		"time":       time.Now().UTC(),
	})
}

func (a *App) adminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Username == "" {
		req.Username = a.Cfg.AdminUser
	}

	if req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	var hash string
	err := a.DB.QueryRow("SELECT password_hash FROM admins WHERE username = ?", req.Username).Scan(&hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "database error"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := a.issueToken(req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "token": token})
}

func (a *App) issueToken(username string) (string, error) {
	claims := jwtClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.Cfg.TokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "meituanone",
			Subject:   "admin",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.Cfg.JWTSecret))
}

func (a *App) adminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			c.Abort()
			return
		}

		tokenStr := authHeader[7:]
		if !a.validateToken(tokenStr) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (a *App) validateToken(tokenStr string) bool {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		return []byte(a.Cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return false
	}
	return true
}

func (a *App) adminEvents(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		authHeader := c.GetHeader("Authorization")
		if len(authHeader) >= 8 && authHeader[:7] == "Bearer " {
			tokenStr = authHeader[7:]
		}
	}

	if !a.validateToken(tokenStr) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "stream unsupported"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 8)
	a.Events.Subscribe(ch)
	defer a.Events.Unsubscribe(ch)

	fmt.Fprintf(c.Writer, "data: %s\n\n", `{"type":"connected"}`)
	flusher.Flush()

	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			flusher.Flush()
		}
	}
}

type menuCategory struct {
	ID       int64         `json:"id"`
	Name     string        `json:"name"`
	Products []menuProduct `json:"products"`
}

type menuProduct struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Price       int    `json:"price"`
	Image       string `json:"image,omitempty"`
	Description string `json:"description,omitempty"`
}

func (a *App) getMenu(c *gin.Context) {
	rows, err := a.DB.Query("SELECT id, name FROM categories WHERE is_active = 1 ORDER BY sort_order ASC, id ASC")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "load categories failed"})
		return
	}
	defer rows.Close()

	result := make([]menuCategory, 0)
	for rows.Next() {
		var cat menuCategory
		if err := rows.Scan(&cat.ID, &cat.Name); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan category failed"})
			return
		}

		prows, err := a.DB.Query(
			"SELECT id, name, price, COALESCE(image, ''), COALESCE(description, '') FROM products WHERE category_id = ? AND is_active = 1 ORDER BY sort_order ASC, id ASC",
			cat.ID,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load products failed"})
			return
		}

		cat.Products = make([]menuProduct, 0)
		for prows.Next() {
			var p menuProduct
			if err := prows.Scan(&p.ID, &p.Name, &p.Price, &p.Image, &p.Description); err != nil {
				prows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "scan product failed"})
				return
			}
			cat.Products = append(cat.Products, p)
		}
		prows.Close()

		result = append(result, cat)
	}

	c.JSON(http.StatusOK, gin.H{"store_name": a.Cfg.StoreName, "categories": result})
}

type createOrderRequest struct {
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
	OrderType     string `json:"order_type"`
	Address       string `json:"address"`
	Note          string `json:"note"`
	Items         []struct {
		ProductID int64 `json:"product_id"`
		Qty       int   `json:"qty"`
	} `json:"items"`
}

func (a *App) createOrder(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "items required"})
		return
	}

	if req.OrderType == "" {
		req.OrderType = "dine_in"
	}

	now := time.Now().UTC()
	orderNo := fmt.Sprintf("O%s%03d", now.Format("20060102150405"), rand.Intn(1000))

	tx, err := a.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "begin tx failed"})
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO orders (order_no, customer_name, customer_phone, order_type, address, note, total_amount, status, print_status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 0, 'pending', 'pending', ?, ?)`,
		orderNo,
		req.CustomerName,
		req.CustomerPhone,
		req.OrderType,
		req.Address,
		req.Note,
		now,
		now,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create order failed"})
		return
	}

	orderID, _ := res.LastInsertId()
	total := 0
	printItems := make([]printer.Item, 0, len(req.Items))

	for _, it := range req.Items {
		if it.Qty <= 0 {
			continue
		}

		var name string
		var price int
		err := tx.QueryRow("SELECT name, price FROM products WHERE id = ? AND is_active = 1", it.ProductID).Scan(&name, &price)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("product %d not found", it.ProductID)})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load product failed"})
			return
		}

		subtotal := price * it.Qty
		total += subtotal

		_, err = tx.Exec(
			`INSERT INTO order_items (order_id, product_id, product_name, unit_price, qty, subtotal, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			orderID,
			it.ProductID,
			name,
			price,
			it.Qty,
			subtotal,
			now,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create order item failed"})
			return
		}

		printItems = append(printItems, printer.Item{Name: name, Qty: it.Qty, Unit: price, Subtotal: subtotal})
	}

	if total <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order total"})
		return
	}

	if _, err := tx.Exec("UPDATE orders SET total_amount = ?, updated_at = ? WHERE id = ?", total, now, orderID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update order total failed"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "commit order failed"})
		return
	}

	a.Events.BroadcastJSON(gin.H{"type": "order_created", "order_id": orderID, "order_no": orderNo})

	if a.Cfg.AutoPrint {
		payload := printer.Payload{
			OrderNo:       orderNo,
			CreatedAt:     now,
			CustomerName:  req.CustomerName,
			CustomerPhone: req.CustomerPhone,
			OrderType:     req.OrderType,
			Address:       req.Address,
			Note:          req.Note,
			TotalAmount:   total,
			Items:         printItems,
		}

		go a.printAsync(orderID, payload)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"order_id":     orderID,
		"order_no":     orderNo,
		"total_amount": total,
	})
}

func (a *App) printAsync(orderID int64, payload printer.Payload) {
	if err := a.Printer.Print(payload); err != nil {
		a.updatePrintStatus(orderID, "failed")
		return
	}
	a.updatePrintStatus(orderID, "printed")
}

func (a *App) updatePrintStatus(orderID int64, status string) {
	_, _ = a.DB.Exec("UPDATE orders SET print_status = ?, updated_at = ? WHERE id = ?", status, time.Now().UTC(), orderID)
	a.Events.BroadcastJSON(gin.H{"type": "order_print", "order_id": orderID, "print_status": status})
}

func (a *App) listOrders(c *gin.Context) {
	status := c.Query("status")

	query := "SELECT id, order_no, customer_name, customer_phone, order_type, address, note, total_amount, status, print_status, created_at, updated_at FROM orders"
	args := []interface{}{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY id DESC LIMIT 100"

	rows, err := a.DB.Query(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list orders failed"})
		return
	}
	defer rows.Close()

	type orderItem struct {
		ProductID   int64  `json:"product_id"`
		ProductName string `json:"product_name"`
		UnitPrice   int    `json:"unit_price"`
		Qty         int    `json:"qty"`
		Subtotal    int    `json:"subtotal"`
	}
	type orderResp struct {
		ID            int64       `json:"id"`
		OrderNo       string      `json:"order_no"`
		CustomerName  string      `json:"customer_name"`
		CustomerPhone string      `json:"customer_phone"`
		OrderType     string      `json:"order_type"`
		Address       string      `json:"address"`
		Note          string      `json:"note"`
		TotalAmount   int         `json:"total_amount"`
		Status        string      `json:"status"`
		PrintStatus   string      `json:"print_status"`
		CreatedAt     time.Time   `json:"created_at"`
		UpdatedAt     time.Time   `json:"updated_at"`
		Items         []orderItem `json:"items"`
	}

	orders := make([]orderResp, 0)
	for rows.Next() {
		var o orderResp
		if err := rows.Scan(
			&o.ID,
			&o.OrderNo,
			&o.CustomerName,
			&o.CustomerPhone,
			&o.OrderType,
			&o.Address,
			&o.Note,
			&o.TotalAmount,
			&o.Status,
			&o.PrintStatus,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "scan order failed"})
			return
		}

		itemRows, err := a.DB.Query("SELECT product_id, product_name, unit_price, qty, subtotal FROM order_items WHERE order_id = ? ORDER BY id ASC", o.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "load order items failed"})
			return
		}

		o.Items = make([]orderItem, 0)
		for itemRows.Next() {
			var it orderItem
			if err := itemRows.Scan(&it.ProductID, &it.ProductName, &it.UnitPrice, &it.Qty, &it.Subtotal); err != nil {
				itemRows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "scan order item failed"})
				return
			}
			o.Items = append(o.Items, it)
		}
		itemRows.Close()

		orders = append(orders, o)
	}

	c.JSON(http.StatusOK, gin.H{"orders": orders})
}

func (a *App) updateOrderStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Status == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status required"})
		return
	}

	_, err = a.DB.Exec("UPDATE orders SET status = ?, updated_at = ? WHERE id = ?", req.Status, time.Now().UTC(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update status failed"})
		return
	}

	a.Events.BroadcastJSON(gin.H{"type": "order_status", "order_id": id, "status": req.Status})

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (a *App) printOrder(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid order id"})
		return
	}

	payload, err := a.loadPrintableOrder(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := a.Printer.Print(payload); err != nil {
		a.updatePrintStatus(id, "failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "print failed"})
		return
	}

	a.updatePrintStatus(id, "printed")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (a *App) loadPrintableOrder(orderID int64) (printer.Payload, error) {
	var payload printer.Payload
	err := a.DB.QueryRow(
		"SELECT order_no, customer_name, customer_phone, order_type, address, note, total_amount, created_at FROM orders WHERE id = ?",
		orderID,
	).Scan(
		&payload.OrderNo,
		&payload.CustomerName,
		&payload.CustomerPhone,
		&payload.OrderType,
		&payload.Address,
		&payload.Note,
		&payload.TotalAmount,
		&payload.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return payload, fmt.Errorf("order not found")
		}
		return payload, fmt.Errorf("load order failed")
	}

	rows, err := a.DB.Query("SELECT product_name, qty, unit_price, subtotal FROM order_items WHERE order_id = ? ORDER BY id ASC", orderID)
	if err != nil {
		return payload, fmt.Errorf("load order items failed")
	}
	defer rows.Close()

	payload.Items = make([]printer.Item, 0)
	for rows.Next() {
		var it printer.Item
		if err := rows.Scan(&it.Name, &it.Qty, &it.Unit, &it.Subtotal); err != nil {
			return payload, fmt.Errorf("scan order items failed")
		}
		payload.Items = append(payload.Items, it)
	}

	return payload, nil
}
