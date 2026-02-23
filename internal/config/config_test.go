package config

import "testing"

func TestLoad_DefaultsLowWriteAndQuietLogging(t *testing.T) {
	t.Setenv("STORAGE_PROFILE", "")
	t.Setenv("ACCESS_LOG", "")
	t.Setenv("GIN_MODE", "")

	cfg := Load()

	if cfg.StorageProfile != "low_write" {
		t.Fatalf("expected STORAGE_PROFILE default to low_write, got %q", cfg.StorageProfile)
	}

	if cfg.AccessLog {
		t.Fatalf("expected ACCESS_LOG default to false")
	}

	if cfg.GinMode != "release" {
		t.Fatalf("expected GIN_MODE default to release, got %q", cfg.GinMode)
	}
}

func TestLoad_OverrideLowWriteSettings(t *testing.T) {
	t.Setenv("STORAGE_PROFILE", "balanced")
	t.Setenv("ACCESS_LOG", "true")
	t.Setenv("GIN_MODE", "debug")

	cfg := Load()

	if cfg.StorageProfile != "balanced" {
		t.Fatalf("expected STORAGE_PROFILE to be balanced, got %q", cfg.StorageProfile)
	}

	if !cfg.AccessLog {
		t.Fatalf("expected ACCESS_LOG to be true")
	}

	if cfg.GinMode != "debug" {
		t.Fatalf("expected GIN_MODE to be debug, got %q", cfg.GinMode)
	}
}
