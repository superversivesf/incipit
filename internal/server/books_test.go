package server

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestListBooks_Empty(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/api/books")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Books   []models.Book `json:"books"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	decodeJSON(t, resp, &body)
	if body.Total != 0 {
		t.Errorf("total = %d, want 0", body.Total)
	}
	if len(body.Books) != 0 {
		t.Errorf("books = %d, want 0", len(body.Books))
	}
}

func TestListBooks_WithData(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Book A", Author: "Author 1", FilePath: "files/1.epub"})
	ts.seedBook(t, models.Book{Title: "Book B", Author: "Author 2", FilePath: "files/2.epub"})

	resp := ts.authedGet(t, "/api/books")
	assertStatus(t, resp, http.StatusOK)

	var body struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
	}
	decodeJSON(t, resp, &body)
	if body.Total != 2 {
		t.Errorf("total = %d, want 2", body.Total)
	}
	if len(body.Books) != 2 {
		t.Errorf("books = %d, want 2", len(body.Books))
	}
}

func TestListBooks_Pagination(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	for i := 0; i < 5; i++ {
		ts.seedBook(t, models.Book{
			Title:    "Book " + strconv.Itoa(i+1),
			Author:   "Author",
			FilePath: "files/" + strconv.Itoa(i+1) + ".epub",
		})
	}

	resp := ts.authedGet(t, "/api/books?page=1&per_page=2")
	var body struct {
		Books   []models.Book `json:"books"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	decodeJSON(t, resp, &body)
	if body.Total != 5 {
		t.Errorf("total = %d, want 5", body.Total)
	}
	if len(body.Books) != 2 {
		t.Errorf("books = %d, want 2", len(body.Books))
	}
}

func TestListBooks_RequiresAuth(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/api/books")
	assertStatus(t, resp, http.StatusUnauthorized)
}

func TestGetBook_Exists(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:       "Leviathan Wakes",
		Author:      "James S. A. Corey",
		Series:      "The Expanse",
		SeriesIndex: 1,
		Pages:       577,
		Rating:      4.5,
		FilePath:    "files/1.epub",
	})

	resp := ts.authedGet(t, bookURL(id))
	assertStatus(t, resp, http.StatusOK)

	var b models.Book
	decodeJSON(t, resp, &b)
	if b.Title != "Leviathan Wakes" {
		t.Errorf("title = %q", b.Title)
	}
	if b.Pages != 577 {
		t.Errorf("pages = %d, want 577", b.Pages)
	}
}

func TestGetBook_NotFound_404(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/api/books/99999")
	assertStatus(t, resp, http.StatusNotFound)
}

func TestUpdateBook(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:    "Original Title",
		Author:   "Original Author",
		FilePath: "files/1.epub",
	})

	resp := ts.authedPutJSON(t, bookURL(id), map[string]interface{}{
		"title":  "Updated Title",
		"author": "Updated Author",
		"series": "New Series",
	})
	assertStatus(t, resp, http.StatusOK)

	resp2 := ts.authedGet(t, bookURL(id))
	var b models.Book
	decodeJSON(t, resp2, &b)
	if b.Title != "Updated Title" {
		t.Errorf("title = %q, want 'Updated Title'", b.Title)
	}
	if b.Series != "New Series" {
		t.Errorf("series = %q, want 'New Series'", b.Series)
	}
}

func TestDeleteBook(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:    "To Delete",
		Author:   "Author",
		FilePath: "files/1.epub",
	})

	resp := ts.authedDelete(t, bookURL(id))
	assertStatus(t, resp, http.StatusNoContent)

	resp2 := ts.authedGet(t, bookURL(id))
	assertStatus(t, resp2, http.StatusNotFound)
}
