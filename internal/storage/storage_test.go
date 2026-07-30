package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveBookFile(t *testing.T) {
	s := New(t.TempDir())

	src := filepath.Join(t.TempDir(), "source.epub")
	os.WriteFile(src, []byte("epub content"), 0644)

	err := s.SaveBookFile(1, src)
	if err != nil {
		t.Fatalf("SaveBookFile failed: %v", err)
	}

	expected := filepath.Join(s.rootDir, "files", "1.epub")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected file at %s, not found", expected)
	}
}

func TestSaveCover(t *testing.T) {
	s := New(t.TempDir())

	err := s.SaveCover(1, []byte("jpeg data"))
	if err != nil {
		t.Fatalf("SaveCover failed: %v", err)
	}

	expected := filepath.Join(s.rootDir, "covers", "1.jpg")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected cover at %s, not found", expected)
	}
}

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	s := New(t.TempDir())
	hash, err := s.HashFile(path)
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}

	expected := "5d41402abc4b2a76b9719d911017c592"
	if hash != expected {
		t.Errorf("expected hash %s, got %s", expected, hash)
	}
}

func TestBookFilePath(t *testing.T) {
	s := New("/data")
	expected := "/data/files/42.epub"
	got := s.BookFilePath(42)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestCoverPath(t *testing.T) {
	s := New("/data")
	expected := "/data/covers/42.jpg"
	got := s.CoverPath(42)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestLazyDirCreation(t *testing.T) {
	s := New(t.TempDir())
	src := filepath.Join(t.TempDir(), "src.epub")
	os.WriteFile(src, []byte("data"), 0644)

	err := s.SaveBookFile(1, src)
	if err != nil {
		t.Fatalf("SaveBookFile should create dirs: %v", err)
	}
}
