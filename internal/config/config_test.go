package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("INCIPIT_DB_PATH")
	os.Unsetenv("INCIPIT_PORT")
	os.Unsetenv("INCIPIT_STORAGE_DIR")

	cfg := Load()

	if cfg.DBPath != "/data/books.db" {
		t.Errorf("expected default DBPath '/data/books.db', got %q", cfg.DBPath)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default Port '8080', got %q", cfg.Port)
	}
	if cfg.StorageDir != "/data" {
		t.Errorf("expected default StorageDir '/data', got %q", cfg.StorageDir)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("INCIPIT_DB_PATH", "/tmp/custom.db")
	t.Setenv("INCIPIT_PORT", "9090")
	t.Setenv("INCIPIT_STORAGE_DIR", "/custom/storage")

	cfg := Load()

	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("expected DBPath '/tmp/custom.db', got %q", cfg.DBPath)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected Port '9090', got %q", cfg.Port)
	}
	if cfg.StorageDir != "/custom/storage" {
		t.Errorf("expected StorageDir '/custom/storage', got %q", cfg.StorageDir)
	}
}
