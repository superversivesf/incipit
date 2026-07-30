package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jason/incipit/internal/models"
)

type progressRequest struct {
	Percentage float64 `json:"percentage"`
	Progress   string  `json:"progress"`
	Device     string  `json:"device"`
}

func (s *Server) syncHealthcheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"state": "OK"})
}

func (s *Server) syncAuth(w http.ResponseWriter, r *http.Request) {
	user := UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func (s *Server) getProgress(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "missing document hash", http.StatusBadRequest)
		return
	}

	user := UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	progress, err := s.DB.GetProgress(hash, user.ID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, progressRequest{
		Percentage: progress.Percentage,
		Progress:   progress.Progress,
		Device:     progress.Device,
	})
}

func (s *Server) putProgress(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "missing document hash", http.StatusBadRequest)
		return
	}

	user := UserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req progressRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	progress := &models.ReadingProgress{
		DocumentHash: hash,
		UserID:       user.ID,
		Percentage:   req.Percentage,
		Progress:     req.Progress,
		Device:       req.Device,
	}

	if err := s.DB.UpsertProgress(progress); err != nil {
		http.Error(w, "error saving progress", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

var _ = models.ReadingProgress{}
