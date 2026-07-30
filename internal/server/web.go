package server

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/epub"
	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/storage"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var templates map[string]*template.Template

func init() {
	pages := map[string]string{
		"index.html":  "templates/index.html",
		"book.html":   "templates/book.html",
		"upload.html": "templates/upload.html",
		"edit.html":   "templates/edit.html",
		"tags.html":   "templates/tags.html",
		"series.html": "templates/series.html",
	}

	templates = make(map[string]*template.Template)
	for name, path := range pages {
		t, err := template.New(name).Funcs(templateFuncs).ParseFS(templateFS, "templates/base.html", path)
		if err != nil {
			panic(err)
		}
		templates[name] = t
	}
}

func staticFileServer() http.Handler {
	sub, _ := fs.Sub(staticFS, "static")
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
	"mul": func(a, b int) int { return a * b },
}

func (s *Server) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	t, ok := templates[name]
	if !ok {
		http.Error(w, "template not found: "+name, http.StatusInternalServerError)
		return
	}

	// Inject CSRF token into data if it's a map
	if m, ok := data.(map[string]interface{}); ok {
		m["CSRFToken"] = csrfTokenFromRequest(r)
	}

	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

func (s *Server) indexPage(w http.ResponseWriter, r *http.Request) {
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	perPage := 20
	q := r.URL.Query().Get("q")

	var books []models.Book
	var total int

	if q != "" {
		results, count, err := s.searcher.Search(r.Context(), q, searchOpts(page, perPage))
		if err != nil {
			books = nil
			total = 0
		} else {
			books = results
			total = count
		}
	} else {
		var err error
		books, total, err = s.DB.ListBooks(perPage, (page-1)*perPage)
		if err != nil {
			books = nil
			total = 0
		}
	}

	data := map[string]interface{}{
		"Books":   books,
		"Total":   total,
		"Page":    page,
		"PerPage": perPage,
		"Query":   q,
	}

	if q != "" {
		data["SearchQuery"] = q
	}

	s.renderTemplate(w, r, "index.html", data)
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
	s.renderTemplate(w, r, "book.html", map[string]interface{}{
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

	coverPath := s.Storage.CoverPath(id)
	if _, err := os.Stat(coverPath); err != nil {
		book, dbErr := s.DB.GetBook(id)
		if dbErr != nil {
			http.NotFound(w, r)
			return
		}
		servePlaceholderCover(w, book.Title, book.Author)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "max-age=31536000")
	http.ServeFile(w, r, coverPath)
}

func servePlaceholderCover(w http.ResponseWriter, title, author string) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=3600")

	displayTitle := title
	if len(displayTitle) > 30 {
		displayTitle = displayTitle[:27] + "..."
	}
	displayAuthor := author
	if len(displayAuthor) > 25 {
		displayAuthor = displayAuthor[:22] + "..."
	}

	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300" viewBox="0 0 200 300">
  <rect width="200" height="300" fill="#2c3e50"/>
  <rect x="10" y="10" width="180" height="280" fill="#34495e" rx="4"/>
  <text x="100" y="130" font-family="serif" font-size="16" fill="#ecf0f1" text-anchor="middle" font-weight="bold">%s</text>
  <text x="100" y="165" font-family="serif" font-size="12" fill="#bdc3c7" text-anchor="middle" font-style="italic">%s</text>
  <text x="100" y="280" font-family="sans-serif" font-size="10" fill="#7f8c8d" text-anchor="middle">Incipit</text>
</svg>`, escapeXML(displayTitle), escapeXML(displayAuthor))

	w.Write([]byte(svg))
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
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
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "error saving file", http.StatusInternalServerError)
		return
	}
	out.Close()

	defer os.Remove(tmpPath)

	meta, err := epub.Parse(tmpPath)
	if err != nil {
		http.Error(w, "error parsing epub: "+err.Error(), http.StatusBadRequest)
		return
	}

	var lookupResult *models.LookupResult
	ctx := r.Context()
	ol := lookup.NewOLClient("https://openlibrary.org")
	gb := lookup.NewGBClient("https://www.googleapis.com")

	if meta.Identifier != "" {
		olResult, _ := ol.LookupByISBN(ctx, meta.Identifier)
		gbResult, _ := gb.LookupByISBN(ctx, meta.Identifier)
		lookupResult = lookup.Merge(olResult, gbResult)
	} else if meta.Title != "" {
		olResult, _ := ol.LookupByTitle(ctx, meta.Title, meta.Creator)
		gbResult, _ := gb.LookupByTitle(ctx, meta.Title, meta.Creator)
		lookupResult = lookup.Merge(olResult, gbResult)
	}

	book := models.MergeMetadata(meta, lookupResult)

	store := storage.New(s.Config.StorageDir)
	hash, err := store.HashFile(tmpPath)
	if err != nil {
		http.Error(w, "error hashing file", http.StatusInternalServerError)
		return
	}
	book.FileHash = hash

	info, err := os.Stat(tmpPath)
	if err == nil {
		book.FileSize = info.Size()
	}

	bookID, err := s.DB.InsertBook(&book)
	if err != nil {
		http.Error(w, "error saving to database", http.StatusInternalServerError)
		return
	}
	book.ID = bookID
	book.FilePath = "files/" + strconv.FormatInt(bookID, 10) + ".epub"
	s.DB.UpdateBook(&book)

	store.SaveBookFile(bookID, tmpPath)

	if lookupResult != nil && lookupResult.CoverURL != "" {
		resp, err := http.Get(lookupResult.CoverURL)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				coverData, _ := io.ReadAll(resp.Body)
				if len(coverData) > 0 {
					store.SaveCover(bookID, coverData)
					book.CoverPath = "covers/" + strconv.FormatInt(bookID, 10) + ".jpg"
					s.DB.UpdateBook(&book)
				}
			}
		}
	}

	http.Redirect(w, r, "/book/"+strconv.FormatInt(bookID, 10), http.StatusSeeOther)
}

func (s *Server) uploadPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, r, "upload.html", map[string]interface{}{})
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

	s.renderTemplate(w, r, "edit.html", map[string]interface{}{
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
	s.renderTemplate(w, r, "tags.html", map[string]interface{}{"Tags": tags})
}

func (s *Server) seriesPage(w http.ResponseWriter, r *http.Request) {
	series, _ := s.DB.ListSeries()
	s.renderTemplate(w, r, "series.html", map[string]interface{}{"Series": series})
}
