package server

import (
	"encoding/xml"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/opds"
)

const opdsPerPage = 50

func opdsPage(r *http.Request) (int, int) {
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	return page, (page - 1) * opdsPerPage
}

func addPaginationLinks(feed *opds.Feed, baseURL string, page, total int) {
	if page > 1 {
		feed.AddLink(opds.RelSelf, baseURL+"?page="+strconv.Itoa(page), opds.TypeAcquisition)
	} else {
		feed.AddLink(opds.RelSelf, baseURL, opds.TypeAcquisition)
	}
	feed.AddLink(opds.RelStart, "/opds", opds.TypeNavigation)
	if page*opdsPerPage < total {
		feed.AddLink(opds.RelNext, baseURL+"?page="+strconv.Itoa(page+1), opds.TypeAcquisition)
	}
}

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
	page, offset := opdsPage(r)
	books, total, _ := s.DB.ListBooks(opdsPerPage, offset)
	feed := opds.NewFeed("urn:incipit:newest", "Newest Books")
	addPaginationLinks(feed, "/opds/newest", page, total)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByAuthor(w http.ResponseWriter, r *http.Request) {
	authors, err := s.DB.DistinctAuthors()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	feed := opds.NewFeed("urn:incipit:byauthor", "By Author")
	feed.AddLink(opds.RelSelf, "/opds/byauthor", opds.TypeNavigation)
	feed.AddLink(opds.RelStart, "/opds", opds.TypeNavigation)
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
	page, offset := opdsPage(r)
	books := s.getBooksByAuthorPaged(author, offset)
	total := s.DB.CountBooksByAuthor(author)
	feed := opds.NewFeed("urn:incipit:byauthor:"+author, "Books by "+author)
	addPaginationLinks(feed, "/opds/byauthor/"+author, page, total)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsBySeries(w http.ResponseWriter, r *http.Request) {
	seriesList, _ := s.DB.ListSeries()
	feed := opds.NewFeed("urn:incipit:byseries", "By Series")
	feed.AddLink(opds.RelSelf, "/opds/byseries", opds.TypeNavigation)
	feed.AddLink(opds.RelStart, "/opds", opds.TypeNavigation)
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
	page, offset := opdsPage(r)
	books := s.getBooksBySeriesPaged(series, offset)
	total := s.DB.CountBooksBySeries(series)
	feed := opds.NewFeed("urn:incipit:byseries:"+series, series)
	addPaginationLinks(feed, "/opds/byseries/"+series, page, total)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsByTag(w http.ResponseWriter, r *http.Request) {
	tags, _ := s.DB.ListTags()
	feed := opds.NewFeed("urn:incipit:bytag", "By Tag")
	feed.AddLink(opds.RelSelf, "/opds/bytag", opds.TypeNavigation)
	feed.AddLink(opds.RelStart, "/opds", opds.TypeNavigation)
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
	page, offset := opdsPage(r)
	books := s.getBooksByTagPaged(tagID, offset)
	total := s.DB.CountBooksByTag(tagID)
	feed := opds.NewFeed("urn:incipit:bytag:"+idStr, "Books with tag")
	addPaginationLinks(feed, "/opds/bytag/"+idStr, page, total)
	for _, b := range books {
		feed.AddEntry(bookToEntry(b))
	}
	s.writeOPDS(w, feed)
}

func (s *Server) opdsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	page, _ := opdsPage(r)
	results, total, _ := s.searcher.Search(r.Context(), q, searchOpts(page, opdsPerPage))
	feed := opds.NewFeed("urn:incipit:search:"+q, "Search: "+q)
	addPaginationLinks(feed, "/opds/search?q="+q, page, total)
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
	_, err = s.DB.GetBook(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	http.ServeFile(w, r, s.Storage.BookFilePath(id))
}

func bookToEntry(b models.Book) opds.Entry {
	entry := opds.Entry{
		ID:     "urn:incipit:book:" + strconv.FormatInt(b.ID, 10),
		Title:  b.Title,
		Author: &opds.Author{Name: b.Author},
		Links: []opds.Link{
			{Rel: opds.RelImage, Href: "/covers/" + strconv.FormatInt(b.ID, 10), Type: opds.TypeJPEG},
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

func (s *Server) getBooksByAuthorPaged(author string, offset int) []models.Book {
	books, _ := s.DB.BooksByAuthor(author, opdsPerPage, offset)
	return books
}

func (s *Server) getBooksBySeriesPaged(series string, offset int) []models.Book {
	books, _ := s.DB.BooksBySeries(series, opdsPerPage, offset)
	return books
}

func (s *Server) getBooksByTagPaged(tagID int64, offset int) []models.Book {
	books, _ := s.DB.BooksByTag(tagID, opdsPerPage, offset)
	return books
}
