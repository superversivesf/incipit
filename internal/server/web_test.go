package server

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestIndexPage(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/")
	assertStatus(t, resp, http.StatusOK)

	body := string(readBody(t, resp))
	if !contains(body, "<title>Incipit Library</title>") {
		t.Error("missing correct title")
	}
	if !contains(body, "/upload") {
		t.Error("missing upload link")
	}
	if !contains(body, "/tags") {
		t.Error("missing tags link")
	}
	if !contains(body, "/series") {
		t.Error("missing series link")
	}
}

func TestIndexPage_WithBooks(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Leviathan Wakes", Author: "Corey", FilePath: "files/1.epub"})

	body := string(readBody(t, ts.authedGet(t, "/")))
	if !contains(body, "Leviathan Wakes") {
		t.Error("book title not in page")
	}
	if !contains(body, "/book/1") {
		t.Error("missing book link")
	}
}

func TestBookPage(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:       "Leviathan Wakes",
		Author:      "James S. A. Corey",
		Series:      "The Expanse",
		SeriesIndex: 1,
		Pages:       577,
		FilePath:    "files/1.epub",
	})

	resp := ts.authedGet(t, "/book/"+strconvI(id))
	assertStatus(t, resp, http.StatusOK)

	body := string(readBody(t, resp))
	if !contains(body, "Leviathan Wakes") {
		t.Error("missing title")
	}
	if !contains(body, "James S. A. Corey") {
		t.Error("missing author")
	}
	if !contains(body, "/book/"+strconvI(id)+"/edit") {
		t.Error("missing edit link")
	}
}

func TestBookPage_NotFound(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/book/99999")
	assertStatus(t, resp, http.StatusNotFound)
}

func TestTagsPage(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/tags")
	assertStatus(t, resp, http.StatusOK)
}

func TestSeriesPage(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/series")
	assertStatus(t, resp, http.StatusOK)
}

func TestUploadPage(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/upload")
	assertStatus(t, resp, http.StatusOK)
}

func TestCoverPlaceholder(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:    "Test Book",
		Author:   "Test Author",
		FilePath: "files/1.epub",
	})

	resp := ts.authedGet(t, "/covers/"+strconvI(id))
	assertStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("content-type = %q, want image/svg+xml", ct)
	}

	body := string(readBody(t, resp))
	if !contains(body, "<svg") {
		t.Error("expected SVG response")
	}
	if !contains(body, "Test Book") {
		t.Error("missing book title in placeholder")
	}
}

func TestCoverNotFound(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/covers/99999")
	assertStatus(t, resp, http.StatusNotFound)
}

func TestStaticCSS(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/static/style.css")
	assertStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if ct != "text/css; charset=utf-8" {
		t.Errorf("content-type = %q, want text/css", ct)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func strconvI(i int64) string {
	return strconv.FormatInt(i, 10)
}
