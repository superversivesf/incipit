package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const csrfCookieName = "incipit_csrf"
const csrfFormField = "_csrf"

// csrfToken generates a random hex token.
func csrfToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// csrfMiddleware sets a CSRF cookie on GET requests and validates the
// token on state-changing requests (POST, PUT, DELETE) for web form routes.
// API routes using basic auth are not vulnerable to CSRF (no cookies),
// so this only applies to browser-submitted HTML forms.
func (s *Server) csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CSRF cookie on every request if not present
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			token := csrfToken()
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
				Secure:   false, // set to true when behind HTTPS
			})
			// Store in a response header so templates can access it
			w.Header().Set("X-CSRF-Token", token)
		} else {
			w.Header().Set("X-CSRF-Token", cookie.Value)
		}

		// Only validate on state-changing methods
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if this is a form submission (form-urlencoded or multipart)
		// or an API call (JSON or no body — protected by basic auth)
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/x-www-form-urlencoded" && contentType != "multipart/form-data" {
			// API call — CSRF not applicable (basic auth, not cookies)
			next.ServeHTTP(w, r)
			return
		}

		// Form submission — validate CSRF token
		r.ParseForm()
		formToken := r.FormValue(csrfFormField)
		cookieToken := ""
		if cookie, err := r.Cookie(csrfCookieName); err == nil {
			cookieToken = cookie.Value
		}

		if formToken == "" || formToken != cookieToken {
			http.Error(w, "CSRF token mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// csrfTokenFromRequest extracts the CSRF token from the cookie for
// use in templates.
func csrfTokenFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}
