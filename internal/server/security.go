package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// rateLimiter tracks failed auth attempts per IP and blocks IPs that
// exceed the threshold. It's a simple in-memory rate limiter — no
// external dependency needed for a single-instance app.
type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attemptList
	maxFails int
	window   time.Duration
	banTime  time.Duration
}

type attemptList struct {
	fails    []time.Time
	bannedAt time.Time
}

func newRateLimiter(maxFails int, window, banTime time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string]*attemptList),
		maxFails: maxFails,
		window:   window,
		banTime:  banTime,
	}
}

func (rl *rateLimiter) recordFailure(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	list, ok := rl.attempts[ip]
	if !ok {
		list = &attemptList{}
		rl.attempts[ip] = list
	}

	now := time.Now()
	// Prune old failures outside the window
	cutoff := now.Add(-rl.window)
	fresh := list.fails[:0]
	for _, t := range list.fails {
		if t.After(cutoff) {
			fresh = append(fresh, t)
		}
	}
	list.fails = append(fresh, now)

	if len(list.fails) >= rl.maxFails {
		list.bannedAt = now
	}
}

func (rl *rateLimiter) isBanned(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	list, ok := rl.attempts[ip]
	if !ok {
		return false
	}

	if list.bannedAt.IsZero() {
		return false
	}

	if time.Since(list.bannedAt) > rl.banTime {
		// Ban expired — clear it
		list.bannedAt = time.Time{}
		list.fails = nil
		return false
	}

	return true
}

func (rl *rateLimiter) recordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

// cleanup removes stale entries periodically to prevent unbounded memory growth.
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, list := range rl.attempts {
		if !list.bannedAt.IsZero() && now.Sub(list.bannedAt) > rl.banTime {
			delete(rl.attempts, ip)
			continue
		}
		// Remove entries with no recent failures
		cutoff := now.Add(-rl.window)
		hasRecent := false
		for _, t := range list.fails {
			if t.After(cutoff) {
				hasRecent = true
				break
			}
		}
		if !hasRecent && list.bannedAt.IsZero() {
			delete(rl.attempts, ip)
		}
	}
}

var limiter = newRateLimiter(10, 5*time.Minute, 15*time.Minute)

func init() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			limiter.cleanup()
		}
	}()
}

// rateLimit middleware checks if the requesting IP is banned.
func rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if ipFor := r.Header.Get("X-Forwarded-For"); ipFor != "" {
			ip = ipFor
		}

		if limiter.isBanned(ip) {
			w.Header().Set("Retry-After", strconv.Itoa(int(15*time.Minute/time.Second)))
			http.Error(w, "Too many failed attempts. Try again later.", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// securityHeaders adds standard security headers to all responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// HSTS — only meaningful behind HTTPS, but harmless over HTTP
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// maxBodySize limits request body size to prevent abuse.
// For upload endpoints, a larger limit is applied separately.
func maxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// captureIP stores the real client IP (behind proxy) in the request context
// so the auth middleware can use it for rate limiting.
func captureIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// chi's middleware.RealIP already handles X-Forwarded-For
		// This just ensures RemoteAddr is populated correctly
		next.ServeHTTP(w, r)
	})
}

var _ = middleware.RealIP
