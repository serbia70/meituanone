package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestApplyStorageProfile_LowWrite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shop.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := ApplyStorageProfile(conn, "low_write"); err != nil {
		t.Fatalf("apply low_write profile: %v", err)
	}

	assertPragmaInt(t, conn, "synchronous", 1)
	assertPragmaInt(t, conn, "temp_store", 2)
	assertPragmaInt(t, conn, "wal_autocheckpoint", 2000)
	assertPragmaInt(t, conn, "busy_timeout", 5000)
	assertPragmaInt(t, conn, "foreign_keys", 1)
}

func TestApplyStorageProfile_Balanced(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shop.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := ApplyStorageProfile(conn, "balanced"); err != nil {
		t.Fatalf("apply balanced profile: %v", err)
	}

	assertPragmaInt(t, conn, "synchronous", 1)
	assertPragmaInt(t, conn, "temp_store", 0)
	assertPragmaInt(t, conn, "wal_autocheckpoint", 1000)
	assertPragmaInt(t, conn, "busy_timeout", 5000)
	assertPragmaInt(t, conn, "foreign_keys", 1)
}

func TestApplyStorageProfile_RejectsUnknownProfile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shop.db")
	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := ApplyStorageProfile(conn, "turbo"); err == nil {
		t.Fatalf("expected error for unknown profile")
	}
}

func assertPragmaInt(t *testing.T, conn *sql.DB, pragma string, want int) {
	t.Helper()

	var got int
	query := "PRAGMA " + pragma
	if err := conn.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %s: %v", pragma, err)
	}

	if got != want {
		t.Fatalf("pragma %s got %d, want %d", pragma, got, want)
	}
}
