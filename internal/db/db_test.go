package db

import (
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var name string
	err = d.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='books'").Scan(&name)
	if err != nil {
		t.Fatalf("books table not found after migrate: %v", err)
	}
	if name != "books" {
		t.Errorf("expected 'books', got %q", name)
	}
}
