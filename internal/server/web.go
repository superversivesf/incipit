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

var templates *template.Template

func init() {
	templates = template.Must(template.New("").ParseFS(templateFS, "templates/*.html"))
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
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
