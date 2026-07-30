package server

import (
	"context"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorized(w)
			return
		}

		user, err := s.DB.GetUser(username)
		if err != nil {
			unauthorized(w)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
			unauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="incipit"`)
	w.WriteHeader(http.StatusUnauthorized)
}
