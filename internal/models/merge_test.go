package models

import "testing"

func TestMergeMetadata(t *testing.T) {
	epub := &Metadata{
		Title:      "Leviathan Wakes",
		Creator:    "James S. A. Corey",
		Identifier: "9780316129084",
		Language:   "en",
		Publisher:  "Orbit Books",
		Date:       "2011-06-15",
	}

	lookup := &LookupResult{
		Title:     "Leviathan Wakes (The Expanse #1)",
		Author:    "James S. A. Corey",
		Series:    "The Expanse",
		Subjects:  []string{"Space warfare", "Fiction"},
		CoverURL:  "https://covers.openlibrary.org/b/id/11295081-L.jpg",
		Pages:     577,
		Publisher: "Orbit Books",
		Published: "2011-06-15",
		Rating:    4.5,
	}

	book := MergeMetadata(epub, lookup)

	if book.ISBN != "9780316129084" {
		t.Errorf("expected ISBN from EPUB, got %q", book.ISBN)
	}
	if book.Title != "Leviathan Wakes (The Expanse #1)" {
		t.Errorf("expected title from lookup, got %q", book.Title)
	}
	if book.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got %q", book.Series)
	}
	if book.Pages != 577 {
		t.Errorf("expected pages 577, got %d", book.Pages)
	}
	if book.Rating != 4.5 {
		t.Errorf("expected rating 4.5, got %f", book.Rating)
	}
}

func TestMergeMetadataLookupNil(t *testing.T) {
	epub := &Metadata{
		Title:      "Test Book",
		Creator:    "Test Author",
		Identifier: "1234567890",
	}

	book := MergeMetadata(epub, nil)

	if book.Title != "Test Book" {
		t.Errorf("expected title from EPUB, got %q", book.Title)
	}
	if book.Author != "Test Author" {
		t.Errorf("expected author from EPUB, got %q", book.Author)
	}
	if book.ISBN != "1234567890" {
		t.Errorf("expected ISBN from EPUB, got %q", book.ISBN)
	}
}

func TestMergeMetadataEpubNil(t *testing.T) {
	lookup := &LookupResult{
		Title:  "From Lookup",
		Author: "Lookup Author",
	}

	book := MergeMetadata(nil, lookup)

	if book.Title != "From Lookup" {
		t.Errorf("expected title from lookup, got %q", book.Title)
	}
	if book.Author != "Lookup Author" {
		t.Errorf("expected author from lookup, got %q", book.Author)
	}
}
