package db

import "testing"

func TestCachePutAndGet(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	err := d.CachePut("9780316129084", "Leviathan Wakes", "Corey", "openlibrary", `{"title":"LW"}`)
	if err != nil {
		t.Fatalf("CachePut failed: %v", err)
	}

	cached, err := d.CacheGet("9780316129084", "openlibrary")
	if err != nil {
		t.Fatalf("CacheGet failed: %v", err)
	}
	if cached != `{"title":"LW"}` {
		t.Errorf("expected cached response, got %q", cached)
	}
}

func TestCacheGetMiss(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	_, err := d.CacheGet("nonexistent", "openlibrary")
	if err == nil {
		t.Fatal("expected error on cache miss, got nil")
	}
}
