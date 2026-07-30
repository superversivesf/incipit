package server

import (
	"embed"
	"html/template"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/go-chi/chi/v5"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var templatePages map[string]*template.Template

func init() {
	pages := map[string]string{
		"index.html":  "templates/index.html",
		"book.html":   "templates/book.html",
		"upload.html": "templates/upload.html",
		"edit.html":   "templates/edit.html",
		"tags.html":   "templates/tags.html",
		"series.html": "templates/series.html",
	}

	templatePages = make(map[string]*template.Template)
	for name, path := range pages {
		t, err := template.New(name).ParseFS(templateFS, "templates/base.html", path)
		if err != nil {
			panic(err)
		}
		templatePages[name] = t
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	t, ok := templatePages[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) indexPage(w http.ResponseWriter, r *http.Request) {
	books, total, _ := s.DB.ListBooks(100, 0)
	s.renderTemplate(w, "index.html", map[string]interface{}{
		"Books": books,
		"Total": total,
	})
}

func (s *Server) bookPage(w http.ResponseWriter, r *http.Request) {
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
	s.renderTemplate(w, "book.html", map[string]interface{}{
		"Book": book,
		"Tags": tags,
	})
}

func (s *Server) serveCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "max-age=31536000")
	http.ServeFile(w, r, s.Storage.CoverPath(id))
}

func (s *Server) serveFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/epub+zip")
	http.ServeFile(w, r, s.Storage.BookFilePath(id))
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(50 << 20)

	file, header, err := r.FormFile("epub")
	if err != nil {
		http.Error(w, "no file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	tmpPath := s.Config.StorageDir + "/tmp_" + header.Filename
	out, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "cannot save upload", http.StatusInternalServerError)
		return
	}
	defer out.Close()
	io.Copy(out, file)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) uploadPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "upload.html", nil)
}

func (s *Server) editBookPage(w http.ResponseWriter, r *http.Request) {
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
	allTags, _ := s.DB.ListTags()
	bookTags, _ := s.DB.GetTagsForBook(id)

	bookTagIDs := map[int64]bool{}
	for _, t := range bookTags {
		bookTagIDs[t.ID] = true
	}

	s.renderTemplate(w, "edit.html", map[string]interface{}{
		"Book":       book,
		"AllTags":    allTags,
		"BookTagIDs": bookTagIDs,
	})
}

func (s *Server) editBookSave(w http.ResponseWriter, r *http.Request) {
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

	r.ParseForm()
	book.Title = r.FormValue("title")
	book.Author = r.FormValue("author")
	book.Series = r.FormValue("series")
	book.ISBN = r.FormValue("isbn")
	book.Description = r.FormValue("description")
	book.Publisher = r.FormValue("publisher")
	book.Published = r.FormValue("published")

	if si := r.FormValue("series_index"); si != "" {
		book.SeriesIndex, _ = strconv.ParseFloat(si, 64)
	} else {
		book.SeriesIndex = 0
	}

	if p := r.FormValue("pages"); p != "" {
		book.Pages, _ = strconv.Atoi(p)
	} else {
		book.Pages = 0
	}

	s.DB.UpdateBook(book)

	// Update tags: remove all existing, add selected
	existingTags, _ := s.DB.GetTagsForBook(id)
	for _, t := range existingTags {
		s.DB.RemoveTagFromBook(id, t.ID)
	}
	for _, tagIDStr := range r.Form["tags"] {
		tagID, _ := strconv.ParseInt(tagIDStr, 10, 64)
		s.DB.AddTagToBook(id, tagID)
	}

	http.Redirect(w, r, "/book/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (s *Server) deleteBookPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.DB.DeleteBook(id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) tagsPage(w http.ResponseWriter, r *http.Request) {
	tags, _ := s.DB.ListTags()
	s.renderTemplate(w, "tags.html", map[string]interface{}{"Tags": tags})
}

func (s *Server) seriesPage(w http.ResponseWriter, r *http.Request) {
	series, _ := s.DB.ListSeries()
	s.renderTemplate(w, "series.html", map[string]interface{}{"Series": series})
}
