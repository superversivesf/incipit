package server

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestOpenSearchDocument_NoAuth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/opds/opensearch.xml")
	assertStatus(t, resp, http.StatusOK)

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "opensearchdescription") {
		t.Errorf("content-type = %q, want opensearchdescription", ct)
	}

	body := string(readBody(t, resp))
	if !strings.Contains(body, "OpenSearchDescription") {
		t.Error("missing OpenSearchDescription element")
	}
	if !strings.Contains(body, "/opds/search?q={searchTerms}") {
		t.Error("missing search URL template")
	}
}

func TestOPDSRoot_HasSearchLink(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/opds")
	assertStatus(t, resp, http.StatusOK)

	feed := parseOPDS(t, resp)
	if findLink(feed.Links, "search") == nil {
		t.Error("missing rel=search link in root feed")
	}
}

func TestIndexPage_ShowsCurrentlyReading(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Seed a book and add progress
	ts.seedBook(t, models.Book{
		Title:    "Leviathan Wakes",
		Author:   "James S. A. Corey",
		FileHash: "testhash123",
		FilePath: "files/1.epub",
	})

	// Save progress for the test user
	ts.database.UpsertProgress(&models.ReadingProgress{
		DocumentHash: "testhash123",
		UserID:       1,
		Percentage:   0.318,
		Progress:     "/body/1",
		Device:       "Kobo",
	})

	resp := ts.authedGet(t, "/")
	assertStatus(t, resp, http.StatusOK)

	body := string(readBody(t, resp))
	if !strings.Contains(body, "Currently Reading") {
		t.Error("missing 'Currently Reading' section")
	}
	if !strings.Contains(body, "Leviathan Wakes") {
		t.Error("missing book title in progress section")
	}
	if !strings.Contains(body, "32%") {
		t.Error("missing progress percentage")
	}
}

func TestIndexPage_NoCurrentlyReadingWhenEmpty(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/")
	assertStatus(t, resp, http.StatusOK)

	body := string(readBody(t, resp))
	if strings.Contains(body, "Currently Reading") {
		t.Error("should not show 'Currently Reading' when no progress exists")
	}
}

func TestIndexPage_ProgressHiddenWhenSearching(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{
		Title:    "Leviathan Wakes",
		Author:   "Corey",
		FileHash: "testhash456",
		FilePath: "files/1.epub",
	})

	ts.database.UpsertProgress(&models.ReadingProgress{
		DocumentHash: "testhash456",
		UserID:       1,
		Percentage:   0.50,
		Progress:     "/body/1",
		Device:       "Kobo",
	})

	resp := ts.authedGet(t, "/?q=levi")
	assertStatus(t, resp, http.StatusOK)

	body := string(readBody(t, resp))
	if strings.Contains(body, "Currently Reading") {
		t.Error("progress section should be hidden during search")
	}
}

var _ = xml.Header
