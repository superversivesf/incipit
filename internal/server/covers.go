package server

import (
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) uploadCover(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid book ID", http.StatusBadRequest)
		return
	}

	r.ParseMultipartForm(10 << 20)

	file, _, err := r.FormFile("cover")
	if err != nil {
		http.Error(w, "no file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	imageData, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "error reading file", http.StatusInternalServerError)
		return
	}

	if len(imageData) < 3 || imageData[0] != 0xFF || imageData[1] != 0xD8 {
		http.Error(w, "file is not a JPEG", http.StatusBadRequest)
		return
	}

	if err := s.Storage.SaveCover(id, imageData); err != nil {
		http.Error(w, "error saving cover", http.StatusInternalServerError)
		return
	}

	book, err := s.DB.GetBook(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}
	book.CoverPath = "covers/" + strconv.FormatInt(id, 10) + ".jpg"
	s.DB.UpdateBook(book)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
