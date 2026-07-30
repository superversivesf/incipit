package server

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestListTags_Empty(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/api/tags")
	assertStatus(t, resp, http.StatusOK)

	var tags []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	decodeJSON(t, resp, &tags)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestCreateTag(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedPostJSON(t, "/api/tags", map[string]interface{}{
		"name": "Science Fiction",
	})
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]int64
	decodeJSON(t, resp, &result)
	if result["id"] == 0 {
		t.Error("expected non-zero ID")
	}
}

func TestUpdateTag(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	createResp := ts.authedPostJSON(t, "/api/tags", map[string]interface{}{"name": "Old Name"})
	var created map[string]int64
	decodeJSON(t, createResp, &created)
	tagID := created["id"]

	resp := ts.authedPutJSON(t, "/api/tags/"+strconv.FormatInt(tagID, 10), map[string]interface{}{
		"name": "New Name",
	})
	assertStatus(t, resp, http.StatusOK)
}

func TestDeleteTag(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	createResp := ts.authedPostJSON(t, "/api/tags", map[string]interface{}{"name": "ToDelete"})
	var created map[string]int64
	decodeJSON(t, createResp, &created)
	tagID := created["id"]

	resp := ts.authedDelete(t, "/api/tags/"+strconv.FormatInt(tagID, 10))
	assertStatus(t, resp, http.StatusOK)
}

func TestListSeries_Empty(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/api/series")
	assertStatus(t, resp, http.StatusOK)

	var series []struct {
		Name      string `json:"name"`
		BookCount int    `json:"book_count"`
	}
	decodeJSON(t, resp, &series)
	if len(series) != 0 {
		t.Errorf("expected 0 series, got %d", len(series))
	}
}

func TestRenameSeries(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "B1", Author: "A", Series: "OldName", FilePath: "f/1.epub"})
	ts.seedBook(t, models.Book{Title: "B2", Author: "A", Series: "OldName", FilePath: "f/2.epub"})

	resp := ts.authedPostJSON(t, "/api/series/rename", map[string]interface{}{
		"old_name": "OldName",
		"new_name": "NewName",
	})
	assertStatus(t, resp, http.StatusOK)

	// Verify via series list
	listResp := ts.authedGet(t, "/api/series")
	var series []struct {
		Name      string `json:"name"`
		BookCount int    `json:"book_count"`
	}
	decodeJSON(t, listResp, &series)
	for _, s := range series {
		if s.Name == "OldName" {
			t.Error("old series name still exists")
		}
		if s.Name == "NewName" && s.BookCount != 2 {
			t.Errorf("NewName count = %d, want 2", s.BookCount)
		}
	}
}

func TestTagsRequireAuth(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/api/tags")
	assertStatus(t, resp, http.StatusUnauthorized)
}
