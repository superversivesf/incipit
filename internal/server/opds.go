package server

import (
	"encoding/xml"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/opds"
)

func (s *Server) writeOPDS(w http.ResponseWriter, feed *opds.Feed) {
	w.Header().Set("Content-Type", "application/atom+xml; profile=opds-catalog")
	data, err := feed.Marshal()
	if err != nil {
		http.Error(w, "xml error", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func (s *Server) opdsRoot(w http.ResponseWriter, r *http.Request) {
	feed := opds.NewFeed("urn:incipit:root", "Incipit Library")
	feed.AddLink(opds.RelSelf, "/opds", opds.TypeNavigation)
	feed.AddLink(opds.RelStart, "/opds", opds.TypeNavigation)

	feed.AddEntry(opds.Entry{
		Title: "Newest Books",
		Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/newest", Type: opds.TypeAcquisition}},
	})
	feed.AddEntry(opds.Entry{
		Title: "By Author",
		Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/byauthor", Type: opds.TypeNavigation}},
	})
	feed.AddEntry(opds.Entry{
		Title: "By Series",
		Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/byseries", Type: opds.TypeNavigation}},
	})
	feed.AddEntry(opds.Entry{
		Title: "By Tag",
		Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/bytag", Type: opds.TypeNavigation}},
	})
	feed.AddEntry(opds.Entry{
		Title: "Search",
		Links: []opds.Link{{Rel: opds.RelSearch, Href: "/opds/search?q={searchTerms}", Type: opds.TypeAcquisition}},
	})

	s.writeOPDS(w, feed)
}

func (s *Server) opdsNewest(w http.ResponseWriter, r *http.Request) {
	books, _, _ := s.DB.ListBooks(50, 0)
	feed := opds.NewFeed("urn:incipit:newest", "Newest Books")
	feed.AddLink(opds.RelSelf, "/opds/newest", opds.TypeAcquisition)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByAuthor(w http.ResponseWriter, r *http.Request) {
	series, _ := s.DB.ListSeries()
	_ = series
	authors, err := s.DB.DistinctAuthors()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	feed := opds.NewFeed("urn:incipit:byauthor", "By Author")
	feed.AddLink(opds.RelSelf, "/opds/byauthor", opds.TypeNavigation)
	for _, a := range authors {
		feed.AddEntry(opds.Entry{
			Title: a,
			Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/byauthor/" + a, Type: opds.TypeAcquisition}},
		})
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByAuthorBooks(w http.ResponseWriter, r *http.Request) {
	author := chi.URLParam(r, "author")
	books, _ := s.DB.BooksByAuthor(author, 50, 0)
	feed := opds.NewFeed("urn:incipit:byauthor:"+author, "Books by "+author)
	feed.AddLink(opds.RelSelf, "/opds/byauthor/"+author, opds.TypeAcquisition)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsBySeries(w http.ResponseWriter, r *http.Request) {
	seriesList, _ := s.DB.ListSeries()
	feed := opds.NewFeed("urn:incipit:byseries", "By Series")
	feed.AddLink(opds.RelSelf, "/opds/byseries", opds.TypeNavigation)
	for _, s := range seriesList {
		feed.AddEntry(opds.Entry{
			Title: s.Name + " (" + strconv.Itoa(s.BookCount) + ")",
			Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/byseries/" + s.Name, Type: opds.TypeAcquisition}},
		})
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsBySeriesBooks(w http.ResponseWriter, r *http.Request) {
	series := chi.URLParam(r, "series")
	books, _ := s.DB.BooksBySeries(series, 50, 0)
	feed := opds.NewFeed("urn:incipit:byseries:"+series, series)
	feed.AddLink(opds.RelSelf, "/opds/byseries/"+series, opds.TypeAcquisition)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByTag(w http.ResponseWriter, r *http.Request) {
	tags, _ := s.DB.ListTags()
	feed := opds.NewFeed("urn:incipit:bytag", "By Tag")
	feed.AddLink(opds.RelSelf, "/opds/bytag", opds.TypeNavigation)
	for _, tag := range tags {
		feed.AddEntry(opds.Entry{
			Title: tag.Name,
			Links: []opds.Link{{Rel: opds.RelSubsection, Href: "/opds/bytag/" + strconv.FormatInt(tag.ID, 10), Type: opds.TypeAcquisition}},
		})
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByTagBooks(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "tag")
	tagID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	books, _ := s.DB.BooksByTag(tagID, 50, 0)
	feed := opds.NewFeed("urn:incipit:bytag:"+idStr, "Books with tag")
	feed.AddLink(opds.RelSelf, "/opds/bytag/"+idStr, opds.TypeAcquisition)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	results, _, _ := s.searcher.Search(r.Context(), q, searchOpts(1, 50))
	feed := opds.NewFeed("urn:incipit:search:"+q, "Search: "+q)
	feed.AddLink(opds.RelSelf, "/opds/search?q="+q, opds.TypeAcquisition)
	for _, b := range results {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsDownload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	book, err := s.DB.GetBook(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	http.ServeFile(w, r, s.Storage.BookFilePath(id))
	_ = book
}

func bookToEntry(b models.Book) opds.Entry {
	entry := opds.Entry{
		ID:     "urn:incipit:book:" + strconv.FormatInt(b.ID, 10),
		Title:  b.Title,
		Author: &opds.Author{Name: b.Author},
		Links: []opds.Link{
			{Rel: opds.RelImage, Href: "/covers/" + strconv.FormatInt(b.ID, 10) + ".jpg", Type: opds.TypeJPEG},
			{Rel: opds.RelAcquisition, Href: "/opds/book/" + strconv.FormatInt(b.ID, 10) + "/download", Type: opds.TypeEPUB},
		},
	}
	if b.Series != "" {
		entry.Categories = append(entry.Categories, opds.Category{Term: b.Series, Label: "series"})
	}
	if b.Description != "" {
		entry.Content = &opds.Content{Type: "text", Body: b.Description}
	}
	return entry
}

var _ = xml.Header
