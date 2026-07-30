package lookup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestParseOLResponse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ol_isbn.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	result, err := ParseOLResponse(data)
	if err != nil {
		t.Fatalf("ParseOLResponse failed: %v", err)
	}

	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
	if result.Author != "James S. A. Corey" {
		t.Errorf("expected author 'James S. A. Corey', got %q", result.Author)
	}
	if result.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got %q", result.Series)
	}
	if result.Publisher != "Orbit Books" {
		t.Errorf("expected publisher 'Orbit Books', got %q", result.Publisher)
	}
	if result.Pages != 577 {
		t.Errorf("expected pages 577, got %d", result.Pages)
	}
	if result.CoverURL != "https://covers.openlibrary.org/b/id/11295081-L.jpg" {
		t.Errorf("expected cover URL, got %q", result.CoverURL)
	}
	if len(result.Subjects) != 2 {
		t.Errorf("expected 2 subjects (excluding series), got %d", len(result.Subjects))
	}
}

func TestOLLookupByISBN(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("testdata", "ol_isbn.json"))

	ts := serveFixture(data)
	defer ts.Close()

	client := NewOLClient(ts.URL)
	result, err := client.LookupByISBN(context.Background(), "9780316129084")
	if err != nil {
		t.Fatalf("LookupByISBN failed: %v", err)
	}
	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
}

func TestParseOLResponseEmpty(t *testing.T) {
	result, err := ParseOLResponse([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseOLResponse on empty object failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty response, got %+v", result)
	}
}

func TestParseGBResponse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "gb_isbn.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	result, err := ParseGBResponse(data)
	if err != nil {
		t.Fatalf("ParseGBResponse failed: %v", err)
	}

	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
	if result.Rating != 4.5 {
		t.Errorf("expected rating 4.5, got %f", result.Rating)
	}
	if result.Description != "Two hundred years after migrating into space..." {
		t.Errorf("expected description, got %q", result.Description)
	}
	if result.Published != "2011-06-15" {
		t.Errorf("expected published '2011-06-15', got %q", result.Published)
	}
	if len(result.Subjects) != 2 {
		t.Errorf("expected 2 categories, got %d", len(result.Subjects))
	}
}

func TestParseGBResponseEmpty(t *testing.T) {
	result, err := ParseGBResponse([]byte(`{"items": []}`))
	if err != nil {
		t.Fatalf("ParseGBResponse on empty items failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty items, got %+v", result)
	}
}

func TestGBLookupByISBN(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("testdata", "gb_isbn.json"))
	ts := serveFixture(data)
	defer ts.Close()

	client := NewGBClient(ts.URL)
	result, err := client.LookupByISBN(context.Background(), "9780316129084")
	if err != nil {
		t.Fatalf("LookupByISBN failed: %v", err)
	}
	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
}

func TestMerge(t *testing.T) {
	ol := &models.LookupResult{
		Title:     "Leviathan Wakes",
		Author:    "James S. A. Corey",
		Series:    "The Expanse",
		Subjects:  []string{"Space warfare", "Fiction"},
		CoverURL:  "https://covers.openlibrary.org/b/id/11295081-L.jpg",
		Pages:     577,
		Publisher: "Orbit Books",
		Sources:   []string{"openlibrary"},
	}

	gb := &models.LookupResult{
		Published:   "2011-06-15",
		Rating:      4.5,
		Description: "Two hundred years after migrating into space...",
		Sources:     []string{"googlebooks"},
	}

	merged := Merge(ol, gb)

	if merged.Title != "Leviathan Wakes" {
		t.Errorf("title: expected 'Leviathan Wakes', got %q", merged.Title)
	}
	if merged.Series != "The Expanse" {
		t.Errorf("series: expected 'The Expanse', got %q", merged.Series)
	}
	if merged.Rating != 4.5 {
		t.Errorf("rating: expected 4.5, got %f", merged.Rating)
	}
	if merged.Description != "Two hundred years after migrating into space..." {
		t.Errorf("description: expected from GB, got %q", merged.Description)
	}
	if merged.Published != "2011-06-15" {
		t.Errorf("published: expected '2011-06-15', got %q", merged.Published)
	}
	if len(merged.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(merged.Sources))
	}
}

func TestMergeNils(t *testing.T) {
	ol := &models.LookupResult{Title: "From OL"}
	gb := &models.LookupResult{Rating: 4.0}

	merged := Merge(ol, gb)
	if merged.Title != "From OL" {
		t.Errorf("expected 'From OL', got %q", merged.Title)
	}
	if merged.Rating != 4.0 {
		t.Errorf("expected rating 4.0, got %f", merged.Rating)
	}

	merged = Merge(nil, gb)
	if merged.Rating != 4.0 {
		t.Errorf("expected rating 4.0 from GB, got %f", merged.Rating)
	}

	merged = Merge(ol, nil)
	if merged.Title != "From OL" {
		t.Errorf("expected 'From OL', got %q", merged.Title)
	}
}
