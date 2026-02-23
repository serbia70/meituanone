package main

import (
	"io"
	"log"
	"net/http"
	"path/filepath"

	"meituanone/internal/config"
	"meituanone/internal/db"
	"meituanone/internal/handlers"
	"meituanone/internal/printer"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()
	gin.SetMode(cfg.GinMode)

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer conn.Close()

	if err := db.ApplyStorageProfile(conn, cfg.StorageProfile); err != nil {
		log.Fatalf("apply storage profile: %v", err)
	}

	if err := db.MigrateAndSeed(conn, cfg); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	printerSvc := printer.New(printer.Config{
		Mode:      cfg.PrinterMode,
		Device:    cfg.PrinterDevice,
		TCPAddr:   cfg.PrinterTCP,
		StoreName: cfg.StoreName,
	})

	r := gin.New()
	if cfg.AccessLog {
		r.Use(gin.Logger())
	} else {
		r.Use(gin.LoggerWithWriter(io.Discard))
	}
	r.Use(gin.Recovery())
	r.Use(corsMiddleware(cfg.CORSOrigin))

	h := handlers.New(conn, cfg, printerSvc)
	h.RegisterRoutes(r)

	r.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(cfg.WebDir, "index.html"))
	})
	r.GET("/admin", func(c *gin.Context) {
		c.File(filepath.Join(cfg.WebDir, "admin.html"))
	})

	addr := ":" + cfg.Port
	log.Printf("MeituanOne started on %s", addr)
	log.Printf("Store: %s", cfg.StoreName)
	log.Printf("Printer mode: %s", cfg.PrinterMode)
	log.Printf("Storage profile: %s", cfg.StorageProfile)
	log.Printf("Access log enabled: %t", cfg.AccessLog)
	if err := r.Run(addr); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func corsMiddleware(origin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
