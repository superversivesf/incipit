package search

import (
	"context"
	"testing"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)

func TestLikeSearcher(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.InsertBook(&models.Book{Title: "Leviathan Wakes", Author: "Corey", FilePath: "files/1.epub"})
	d.InsertBook(&models.Book{Title: "Caliban's War", Author: "Corey", FilePath: "files/2.epub"})
	d.InsertBook(&models.Book{Title: "Dune", Author: "Herbert", FilePath: "files/3.epub"})

	s := NewLikeSearcher(d)
	books, total, err := s.Search(context.Background(), "levi", Opts{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result, got %d", total)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].Title != "Leviathan Wakes" {
		t.Errorf("expected 'Leviathan Wakes', got %q", books[0].Title)
	}
}

func TestLikeSearcherEmpty(t *testing.T) {
	d, _ := db.Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	s := NewLikeSearcher(d)
	books, total, err := s.Search(context.Background(), "", Opts{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 results for empty query, got %d", total)
	}
	_ = books
}
