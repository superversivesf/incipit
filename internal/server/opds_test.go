package server

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/jason/incipit/internal/models"
)

type opdsFeed struct {
	XMLName xml.Name    `xml:"feed"`
	ID      string      `xml:"id"`
	Title   string      `xml:"title"`
	Links   []opdsLink  `xml:"link"`
	Entries []opdsEntry `xml:"entry"`
}

type opdsLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
}

type opdsEntry struct {
	ID    string     `xml:"id"`
	Title string     `xml:"title"`
	Links []opdsLink `xml:"link"`
}

func parseOPDS(t *testing.T, resp *http.Response) opdsFeed {
	t.Helper()
	var feed opdsFeed
	if err := xml.NewDecoder(resp.Body).Decode(&feed); err != nil {
		t.Fatalf("parseOPDS: %v", err)
	}
	resp.Body.Close()
	return feed
}

func findLink(links []opdsLink, rel string) *opdsLink {
	for i := range links {
		if links[i].Rel == rel {
			return &links[i]
		}
	}
	return nil
}

func TestOPDSRoot(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/opds")
	assertStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Error("missing Content-Type")
	}

	feed := parseOPDS(t, resp)
	if feed.ID != "urn:incipit:root" {
		t.Errorf("id = %q, want 'urn:incipit:root'", feed.ID)
	}
	if feed.Title != "Incipit Library" {
		t.Errorf("title = %q", feed.Title)
	}

	if findLink(feed.Links, "self") == nil {
		t.Error("missing rel=self link")
	}
	if findLink(feed.Links, "start") == nil {
		t.Error("missing rel=start link")
	}

	titles := map[string]bool{}
	for _, e := range feed.Entries {
		titles[e.Title] = true
	}
	for _, want := range []string{"Newest Books", "By Author", "By Series", "By Tag", "Search"} {
		if !titles[want] {
			t.Errorf("missing entry: %q", want)
		}
	}
}

func TestOPDSNewest_Empty(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.authedGet(t, "/opds/newest")
	assertStatus(t, resp, http.StatusOK)

	feed := parseOPDS(t, resp)
	if findLink(feed.Links, "self") == nil {
		t.Error("missing rel=self link")
	}
	if findLink(feed.Links, "start") == nil {
		t.Error("missing rel=start link")
	}
	if len(feed.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(feed.Entries))
	}
}

func TestOPDSNewest_WithBooks(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Book A", Author: "Author 1", FilePath: "files/1.epub"})
	ts.seedBook(t, models.Book{Title: "Book B", Author: "Author 2", FilePath: "files/2.epub"})

	resp := ts.authedGet(t, "/opds/newest")
	assertStatus(t, resp, http.StatusOK)

	feed := parseOPDS(t, resp)
	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}

	for _, e := range feed.Entries {
		if findLink(e.Links, "http://opds-spec.org/acquisition") == nil {
			t.Errorf("entry %q missing acquisition link", e.Title)
		}
		if findLink(e.Links, "http://opds-spec.org/image") == nil {
			t.Errorf("entry %q missing image link", e.Title)
		}
	}
}

func TestOPDSByAuthor(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Book A", Author: "Corey", FilePath: "files/1.epub"})
	ts.seedBook(t, models.Book{Title: "Book B", Author: "Herbert", FilePath: "files/2.epub"})

	resp := ts.authedGet(t, "/opds/byauthor")
	assertStatus(t, resp, http.StatusOK)

	feed := parseOPDS(t, resp)
	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}
}

func TestOPDSSearch(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	ts.seedBook(t, models.Book{Title: "Leviathan Wakes", Author: "Corey", FilePath: "files/1.epub"})
	ts.seedBook(t, models.Book{Title: "Dune", Author: "Herbert", FilePath: "files/2.epub"})

	resp := ts.authedGet(t, "/opds/search?q=levi")
	assertStatus(t, resp, http.StatusOK)

	feed := parseOPDS(t, resp)
	if len(feed.Entries) != 1 {
		t.Errorf("entries = %d, want 1", len(feed.Entries))
	}
	if feed.Entries[0].Title != "Leviathan Wakes" {
		t.Errorf("title = %q, want 'Leviathan Wakes'", feed.Entries[0].Title)
	}
}

func TestOPDSRequiresAuth(t *testing.T) {
	ts := newTestServerWithUser(t)
	defer ts.Close()

	resp := ts.unauthedRequest(t, "GET", "/opds")
	assertStatus(t, resp, http.StatusUnauthorized)
}
