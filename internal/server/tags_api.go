package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (s *Server) apiCreateTag(w http.ResponseWriter, r *http.Request) {
	var tag struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	id, err := s.DB.CreateTag(tag.Name, tag.ParentID)
	if err != nil {
		http.Error(w, "error creating tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) apiUpdateTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tag ID", http.StatusBadRequest)
		return
	}
	var tag struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.DB.UpdateTag(id, tag.Name, tag.ParentID); err != nil {
		http.Error(w, "error updating tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) apiDeleteTag(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid tag ID", http.StatusBadRequest)
		return
	}
	if err := s.DB.DeleteTag(id); err != nil {
		http.Error(w, "error deleting tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) apiRenameSeries(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.OldName == "" || req.NewName == "" {
		http.Error(w, "old_name and new_name required", http.StatusBadRequest)
		return
	}
	if err := s.DB.RenameSeries(req.OldName, req.NewName); err != nil {
		http.Error(w, "error renaming series", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
