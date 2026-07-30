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
		ip := clientIP(r)

		// Check rate limit before attempting auth
		if limiter.isBanned(ip) {
			w.Header().Set("Retry-After", "900")
			http.Error(w, "Too many failed attempts. Try again later.", http.StatusTooManyRequests)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorized(w)
			return
		}

		user, err := s.DB.GetUser(username)
		if err != nil {
			limiter.recordFailure(ip)
			unauthorized(w)
			return
		}

		// Try the password as-is first (KOReader sends md5(password)).
		// If that fails, MD5-hash it and try again (browsers send plaintext).
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
			md5sum := md5.Sum([]byte(password))
			md5hex := hex.EncodeToString(md5sum[:])
			if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(md5hex)) != nil {
				limiter.recordFailure(ip)
				unauthorized(w)
				return
			}
		}

		limiter.recordSuccess(ip)
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="incipit"`)
	w.WriteHeader(http.StatusUnauthorized)
}
