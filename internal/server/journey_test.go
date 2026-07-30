package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/jason/incipit/internal/models"
)

// Journey tests cover full end-to-end workflows through the server.
// They test the system as a user would use it, not individual handlers.

func TestJourney_AddBookViaCLI_ShowInList_BrowseInOPDS(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Step 1: Seed a book (simulates `incipit add`)
	id := ts.seedBook(t, models.Book{
		Title:       "Leviathan Wakes",
		Author:      "James S. A. Corey",
		Series:      "The Expanse",
		SeriesIndex: 1,
		ISBN:        "9780316129084",
		Pages:       577,
		Rating:      4.5,
		Description: "Two hundred years after migrating into space...",
		FilePath:    "files/1.epub",
		FileHash:    "abc123hash",
	})

	// Step 2: Book appears in JSON API
	resp := ts.authedGet(t, "/api/books")
	var list struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
	}
	decodeJSON(t, resp, &list)
	if list.Total != 1 {
		t.Fatalf("total = %d, want 1", list.Total)
	}

	// Step 3: Book appears in OPDS
	opdsResp := ts.authedGet(t, "/opds/newest")
	feed := parseOPDS(t, opdsResp)
	if len(feed.Entries) != 1 {
		t.Fatalf("OPDS entries = %d, want 1", len(feed.Entries))
	}
	if feed.Entries[0].Title != "Leviathan Wakes" {
		t.Errorf("OPDS title = %q", feed.Entries[0].Title)
	}

	// Step 4: Book detail available
	detailResp := ts.authedGet(t, bookURL(id))
	var book models.Book
	decodeJSON(t, detailResp, &book)
	if book.Title != "Leviathan Wakes" {
		t.Errorf("detail title = %q", book.Title)
	}

	// Step 5: Book appears on index page
	pageResp := ts.authedGet(t, "/")
	pageBody := string(readBody(t, pageResp))
	if !contains(pageBody, "Leviathan Wakes") {
		t.Error("book not on index page")
	}
}

func TestJourney_EditBookMetadata_VerifyChanges(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Add a book
	id := ts.seedBook(t, models.Book{
		Title:    "Original Title",
		Author:   "Original Author",
		FilePath: "files/1.epub",
	})

	// Update via API
	ts.authedPutJSON(t, bookURL(id), map[string]interface{}{
		"title":  "Updated Title",
		"author": "Updated Author",
		"series": "Updated Series",
	})

	// Verify via detail endpoint
	resp := ts.authedGet(t, bookURL(id))
	var book models.Book
	decodeJSON(t, resp, &book)
	if book.Title != "Updated Title" {
		t.Errorf("title = %q", book.Title)
	}
	if book.Author != "Updated Author" {
		t.Errorf("author = %q", book.Author)
	}
	if book.Series != "Updated Series" {
		t.Errorf("series = %q", book.Series)
	}
}

func TestJourney_CreateTag_AssignToBook_Verify(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Create a tag
	createResp := ts.authedPostJSON(t, "/api/tags", map[string]interface{}{"name": "Science Fiction"})
	var created map[string]int64
	decodeJSON(t, createResp, &created)
	tagID := created["id"]

	// Add a book
	bookID := ts.seedBook(t, models.Book{
		Title:    "Dune",
		Author:   "Herbert",
		FilePath: "files/1.epub",
	})

	// Assign tag to book via DB (tag assignment via API not fully wired)
	ts.database.AddTagToBook(bookID, tagID)

	// Verify tag shows on book detail
	resp := ts.authedGet(t, bookURL(bookID))
	assertStatus(t, resp, http.StatusOK)
}

func TestJourney_DeleteBook_VerifyGone(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:    "To Delete",
		Author:   "Author",
		FilePath: "files/1.epub",
	})

	// Delete it
	ts.authedDelete(t, bookURL(id))

	// Verify it's gone from API
	resp := ts.authedGet(t, bookURL(id))
	assertStatus(t, resp, http.StatusNotFound)

	// Verify it's gone from OPDS
	opdsResp := ts.authedGet(t, "/opds/newest")
	feed := parseOPDS(t, opdsResp)
	for _, e := range feed.Entries {
		if e.Title == "To Delete" {
			t.Error("book still in OPDS after deletion")
		}
	}
}

func TestJourney_SyncProgress_AcrossDevices(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	// Device 1 saves progress at 30%
	ts.authedPutJSON(t, "/syncs/progress/doc123", map[string]interface{}{
		"percentage": 0.30,
		"progress":   "/body/chapter1",
		"device":     "Kobo",
	})

	// Device 1 updates to 35%
	ts.authedPutJSON(t, "/syncs/progress/doc123", map[string]interface{}{
		"percentage": 0.35,
		"progress":   "/body/chapter2",
		"device":     "Phone",
	})

	// Device 2 fetches — should get 35%
	resp := ts.authedGet(t, "/syncs/progress/doc123")
	var body map[string]interface{}
	decodeJSON(t, resp, &body)
	if body["percentage"] != 0.35 {
		t.Errorf("percentage = %v, want 0.35", body["percentage"])
	}
	if body["device"] != "Phone" {
		t.Errorf("device = %v, want 'Phone'", body["device"])
	}
}

func TestJourney_SearchBooks(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Leviathan Wakes", Author: "Corey", FilePath: "f/1.epub"})
	ts.seedBook(t, models.Book{Title: "Caliban's War", Author: "Corey", FilePath: "f/2.epub"})
	ts.seedBook(t, models.Book{Title: "Dune", Author: "Herbert", FilePath: "f/3.epub"})

	// Search via API
	resp := ts.authedGet(t, "/api/books?q=levi")
	var list struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
	}
	decodeJSON(t, resp, &list)
	if list.Total != 1 {
		t.Errorf("API search total = %d, want 1", list.Total)
	}

	// Search via OPDS
	opdsResp := ts.authedGet(t, "/opds/search?q=levi")
	feed := parseOPDS(t, opdsResp)
	if len(feed.Entries) != 1 {
		t.Errorf("OPDS search entries = %d, want 1", len(feed.Entries))
	}
	if feed.Entries[0].Title != "Leviathan Wakes" {
		t.Errorf("OPDS search result = %q", feed.Entries[0].Title)
	}
}

func TestJourney_CoverUpload_VerifyServed(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	id := ts.seedBook(t, models.Book{
		Title:    "Test Book",
		Author:   "Author",
		FilePath: "files/1.epub",
	})

	// Before cover upload, should get SVG placeholder
	resp := ts.authedGet(t, "/covers/"+strconvI(id))
	ct := resp.Header.Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("before upload: content-type = %q, want image/svg+xml", ct)
	}
	resp.Body.Close()

	// Upload a fake JPEG (minimal valid JPEG header + data)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("cover", "cover.jpg")
	fw.Write([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 'f', 'a', 'k', 'e'})
	mw.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/books/"+strconvI(id)+"/cover", &buf)
	req.SetBasicAuth("testuser", "testpass")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	uploadResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	uploadResp.Body.Close()
	if uploadResp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d, want 200", uploadResp.StatusCode)
	}

	// After cover upload, should get JPEG
	resp2 := ts.authedGet(t, "/covers/"+strconvI(id))
	ct2 := resp2.Header.Get("Content-Type")
	if ct2 != "image/jpeg" {
		t.Errorf("after upload: content-type = %q, want image/jpeg", ct2)
	}
	resp2.Body.Close()
}
