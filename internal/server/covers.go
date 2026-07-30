package server

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) saveCoverFromUpload(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid book ID", http.StatusBadRequest)
		return 0, false
	}

	r.ParseMultipartForm(10 << 20)

	file, _, err := r.FormFile("cover")
	if err != nil {
		http.Error(w, "no file uploaded", http.StatusBadRequest)
		return 0, false
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "error reading file", http.StatusInternalServerError)
		return 0, false
	}

	if len(imageData) < 3 || imageData[0] != 0xFF || imageData[1] != 0xD8 {
		http.Error(w, "file is not a JPEG", http.StatusBadRequest)
		return 0, false
	}

	if err := s.Storage.SaveCover(id, imageData); err != nil {
		http.Error(w, "error saving cover", http.StatusInternalServerError)
		return 0, false
	}

	book, err := s.DB.GetBook(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return 0, false
	}
	book.CoverPath = "covers/" + strconv.FormatInt(id, 10)
	s.DB.UpdateBook(book)

	return id, true
}

func (s *Server) uploadCover(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.saveCoverFromUpload(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) uploadCoverRedirect(w http.ResponseWriter, r *http.Request) {
	id, ok := s.saveCoverFromUpload(w, r)
	if !ok {
		return
	}
	http.Redirect(w, r, "/book/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
