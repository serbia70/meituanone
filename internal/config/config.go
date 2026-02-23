package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port           string
	WebDir         string
	DBPath         string
	StorageProfile string
	AccessLog      bool
	GinMode        string
	StoreName      string
	AdminUser      string
	AdminPassword  string
	JWTSecret      string
	TokenTTL       time.Duration
	AutoPrint      bool
	PrinterMode    string
	PrinterDevice  string
	PrinterTCP     string
	CORSOrigin     string
}

func Load() Config {
	ttlHours := getEnvInt("TOKEN_TTL_HOURS", 720)

	return Config{
		Port:           getEnv("PORT", "3000"),
		WebDir:         getEnv("WEB_DIR", "./web"),
		DBPath:         getEnv("DB_PATH", "./data/shop.db"),
		StorageProfile: getStorageProfile(getEnv("STORAGE_PROFILE", "low_write")),
		AccessLog:      getEnvBool("ACCESS_LOG", false),
		GinMode:        getEnv("GIN_MODE", "release"),
		StoreName:      getEnv("STORE_NAME", "Demo Store"),
		AdminUser:      getEnv("ADMIN_USER", "admin"),
		AdminPassword:  getEnv("ADMIN_PASSWORD", "admin123"),
		JWTSecret:      getEnv("JWT_SECRET", "replace-me-in-production"),
		TokenTTL:       time.Duration(ttlHours) * time.Hour,
		AutoPrint:      getEnvBool("AUTO_PRINT", true),
		PrinterMode:    getEnv("PRINTER_MODE", "stdout"),
		PrinterDevice:  getEnv("PRINTER_DEVICE", "/dev/usb/lp0"),
		PrinterTCP:     getEnv("PRINTER_TCP", ""),
		CORSOrigin:     getEnv("CORS_ORIGIN", "*"),
	}
}

func getStorageProfile(value string) string {
	switch value {
	case "balanced", "low_write":
		return value
	default:
		return "low_write"
	}
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
