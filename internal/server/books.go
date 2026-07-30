package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/models"
)

type booksResponse struct {
	Books   []bookSummary `json:"books"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}

type bookSummary struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Series      string  `json:"series,omitempty"`
	SeriesIndex float64 `json:"series_index,omitempty"`
	Cover       string  `json:"cover,omitempty"`
}

type bookDetail struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	TitleSort   string    `json:"title_sort"`
	Author      string    `json:"author"`
	AuthorSort  string    `json:"author_sort"`
	Series      string    `json:"series,omitempty"`
	SeriesIndex float64   `json:"series_index,omitempty"`
	ISBN        string    `json:"isbn,omitempty"`
	Description string    `json:"description,omitempty"`
	Publisher   string    `json:"publisher,omitempty"`
	Published   string    `json:"published,omitempty"`
	Pages       int       `json:"pages,omitempty"`
	Rating      float64   `json:"rating,omitempty"`
	Cover       string    `json:"cover,omitempty"`
	FileSize    int64     `json:"file_size,omitempty"`
	Added       string    `json:"added,omitempty"`
	Tags        []tagJSON `json:"tags,omitempty"`
}

type tagJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	perPage := atoiDefault(r.URL.Query().Get("per_page"), 20)
	series := r.URL.Query().Get("series")
	author := r.URL.Query().Get("author")
	q := r.URL.Query().Get("q")

	var books []models.Book
	var total int
	var err error

	if q != "" {
		searcher := s.searcher
		results, count, sErr := searcher.Search(r.Context(), q, searchOpts(page, perPage))
		if sErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		books = results
		total = count
	} else if series != "" {
		books, total = s.DB.CountedBooksBy("series", series, perPage, (page-1)*perPage)
	} else if author != "" {
		books, total = s.DB.CountedBooksBy("author", author, perPage, (page-1)*perPage)
	} else {
		books, total, err = s.DB.ListBooks(perPage, (page-1)*perPage)
	}

	_ = err

	summaries := make([]bookSummary, len(books))
	for i, b := range books {
		summaries[i] = bookSummary{
			ID:          b.ID,
			Title:       b.Title,
			Author:      b.Author,
			Series:      b.Series,
			SeriesIndex: b.SeriesIndex,
			Cover:       coverURL(b.ID),
		}
	}

	writeJSON(w, http.StatusOK, booksResponse{
		Books:   summaries,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	})
}

func searchOpts(page, perPage int) struct{ Limit, Offset int } {
	return struct{ Limit, Offset int }{Limit: perPage, Offset: (page - 1) * perPage}
}

func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
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

	tags, _ := s.DB.GetTagsForBook(id)
	tagJSONs := make([]tagJSON, len(tags))
	for i, tag := range tags {
		tagJSONs[i] = tagJSON{ID: tag.ID, Name: tag.Name}
	}

	writeJSON(w, http.StatusOK, bookDetail{
		ID:          book.ID,
		Title:       book.Title,
		TitleSort:   book.TitleSort,
		Author:      book.Author,
		AuthorSort:  book.AuthorSort,
		Series:      book.Series,
		SeriesIndex: book.SeriesIndex,
		ISBN:        book.ISBN,
		Description: book.Description,
		Publisher:   book.Publisher,
		Published:   book.Published,
		Pages:       book.Pages,
		Rating:      book.Rating,
		Cover:       coverURL(book.ID),
		FileSize:    book.FileSize,
		Added:       book.Added,
		Tags:        tagJSONs,
	})
}

type bookUpdate struct {
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	Series      string   `json:"series"`
	SeriesIndex float64  `json:"series_index"`
	Description string   `json:"description"`
	Rating      float64  `json:"rating"`
	Tags        []string `json:"tags"`
}

func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var update bookUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	book, err := s.DB.GetBook(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	book.Title = update.Title
	book.Author = update.Author
	book.Series = update.Series
	book.SeriesIndex = update.SeriesIndex
	book.Description = update.Description
	book.Rating = update.Rating

	if err := s.DB.UpdateBook(book); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	s.handleGetBook(w, r)
}

func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := s.DB.DeleteBook(id); err != nil {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func coverURL(bookID int64) string {
	return "/covers/" + strconv.FormatInt(bookID, 10)
}
