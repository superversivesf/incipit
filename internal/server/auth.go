package server

import (
	"context"
	"crypto/md5"
	"encoding/hex"
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

		// Try the password as-is first (KOReader sends md5(password)).
		// If that fails, MD5-hash it and try again (browsers send plaintext).
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			md5sum := md5.Sum([]byte(password))
			md5hex := hex.EncodeToString(md5sum[:])
			if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(md5hex)) != nil {
				unauthorized(w)
				return
			}
		}

		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="incipit"`)
	w.WriteHeader(http.StatusUnauthorized)
}
