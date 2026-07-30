package server

import (
	"net/http"

	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
)

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.DB.ListTags()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.DB.ListSeries()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, series)
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	isbn := r.URL.Query().Get("isbn")
	title := r.URL.Query().Get("title")
	author := r.URL.Query().Get("author")

	ctx := r.Context()
	var olResult, gbResult *models.LookupResult

	ol := lookup.NewOLClient("https://openlibrary.org")
	gb := lookup.NewGBClient("https://www.googleapis.com")

	if isbn != "" {
		olResult, _ = ol.LookupByISBN(ctx, isbn)
		gbResult, _ = gb.LookupByISBN(ctx, isbn)
	} else if title != "" {
		olResult, _ = ol.LookupByTitle(ctx, title, author)
		gbResult, _ = gb.LookupByTitle(ctx, title, author)
	}

	merged := lookup.Merge(olResult, gbResult)
	if merged == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, merged)
}
