# Incipit Phase 2: Web Server + OPDS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform the Incipit CLI tool into a web server with JSON API, web UI, and OPDS catalog for KOReader browsing and downloading.

**Architecture:** Phase 2 adds `internal/server` (composition root wiring db+storage+lookup+opds+search into HTTP via chi) and `internal/opds` (pure XML feed generation). Basic auth middleware wraps all routes except health checks. Web UI is server-rendered `html/template` embedded via `embed.FS`.

**Tech Stack:** Go 1.22, `github.com/go-chi/chi/v5` (HTTP router), `github.com/go-chi/cors` (CORS middleware), `modernc.org/sqlite`, `html/template` (stdlib), `embed.FS` (stdlib).

## Global Constraints

- Go 1.22, module path `github.com/jason/incipit`
- Pure-Go SQLite via `modernc.org/sqlite` — no CGO.
- Dependency allowlist: `modernc.org/sqlite`, `github.com/go-chi/chi/v5`, `github.com/go-chi/cors`, plus Go stdlib only.
- HTTP router: chi. Web UI: server-rendered `html/template` — no SPA, no JS framework, no CSS framework.
- All app code under `internal/`. Web assets under `web/{templates,static}/`.
- `opds` depends only on `models`. `server` depends on all.
- Auth: every endpoint requires basic auth EXCEPT `/health` and `/syncs/healthcheck`.
- `web/templates/` and `web/static/` embedded via `embed.FS`.
- OPDS content types: navigation `application/atom+xml; profile=opds-catalog; kind=navigation`, acquisition `application/atom+xml; profile=opds-catalog; kind=acquisition`.
- OPDS pagination: 50 entries per feed, `?page=N`, `<link rel="next">`.
- HTTP integration tests via `httptest`.
- Quality gates: `go vet ./...` clean, `gofmt -l .` empty, `go test ./...` passing.

---

## Phase 1 Interfaces (assumed complete)

Phase 2 consumes these interfaces produced by Phase 1. If signatures differ
from what Phase 1 actually produced, adjust the consuming code to match —
Phase 1 is the source of truth for these signatures.

### internal/models

```go
package models

type Book struct {
    ID          int64
    Title       string
    TitleSort   string
    Author      string
    AuthorSort  string
    Series      string
    SeriesIndex float64
    ISBN        string
    Description  string
    Publisher   string
    Published   string
    Pages        int
    Rating      float64
    CoverPath   string
    FilePath    string
    FileHash    string
    FileSize    int64
    Added       string
    Updated     string
    Tags        []Tag
}

type Tag struct {
    ID       int64
    Name     string
    ParentID int64 // 0 = top-level
}

type User struct {
    ID           int64
    Username      string
    PasswordHash string
    Role         string
    Created      string
}

type Metadata struct { // EPUB metadata
    Title      string
    Creator    string
    Identifier string
    Language   string
    Publisher  string
    Date       string
}

type LookupResult struct {
    Title       string
    Author      string
    Series      string
    Subjects    []string
    CoverURL    string
    Pages       int
    Publisher   string
    Published   string
    Rating      float64
    Description string
    Sources     []string
}

func SortTitle(s string) string
```

### internal/db

```go
package db

type DB struct { /* wraps *sql.DB */ }

func Open(path string) (*DB, error)
func (db *DB) Migrate() error
func (db *DB) Close() error

func (db *DB) GetUser(ctx context.Context, username string) (*models.User, error)
func (db *DB) InsertBook(ctx context.Context, b *models.Book) (int64, error)
func (db *DB) GetBook(ctx context.Context, id int64) (*models.Book, error)
func (db *DB) ListBooks(ctx context.Context, opts ListOpts) ([]models.Book, int, error)
func (db *DB) UpdateBook(ctx context.Context, b *models.Book) error
func (db *DB) DeleteBook(ctx context.Context, id int64) error
func (db *DB) ListTags(ctx context.Context) ([]models.Tag, error)
func (db *DB) GetTagsForBook(ctx context.Context, bookID int64) ([]models.Tag, error)
func (db *DB) AddTagsToBook(ctx context.Context, bookID int64, tagNames []string) error
func (db *DB) ListSeries(ctx context.Context) ([]SeriesInfo, error)
func (db *DB) ListBooksByAuthor(ctx context.Context, author string, limit, offset int) ([]models.Book, int, error)
func (db *DB) ListBooksBySeries(ctx context.Context, series string, limit, offset int) ([]models.Book, int, error)
func (db *DB) ListBooksByTag(ctx context.Context, tag string, limit, offset int) ([]models.Book, int, error)
func (db *DB) GetBookByFileHash(ctx context.Context, hash string) (*models.Book, error)

type ListOpts struct {
    Page    int
    PerPage int
    Series  string
    Author  string
    Tag     string
    Query   string
    Sort    string // "added", "title", "author", "series"
}

type SeriesInfo struct {
    Name      string
    BookCount int
}
```

### internal/epub

```go
package epub

func Parse(path string) (*models.Metadata, error)
func ParseOPF(r io.Reader) (*models.Metadata, error)
```

### internal/lookup

```go
package lookup

func Lookup(ctx context.Context, isbn, title, author string) (*models.LookupResult, error)
```

### internal/storage

```go
package storage

type Storage struct { /* holds root dir path */ }

func New(rootDir string) *Storage
func (s *Storage) SaveBookFile(bookID int64, sourcePath string) error
func (s *Storage) SaveCover(bookID int64, imageData []byte) error
func (s *Storage) BookFilePath(bookID int64) string
func (s *Storage) CoverPath(bookID int64) string
func (s *Storage) DeleteBookFile(bookID int64) error
func (s *Storage) DeleteCover(bookID int64) error
func (s *Storage) HashFile(path string) (string, error)
```

### internal/search

```go
package search

type Searcher interface {
    Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error)
}

type Opts struct {
    Limit  int
    Offset int
}

type LikeSearcher struct { /* implements Searcher */ }
```

### internal/config

```go
package config

type Config struct {
    DBPath     string
    Port       int
    StorageDir string
}

func Load() Config
```

---

## File Structure

Phase 2 creates or modifies these files. Each has one clear responsibility.

| File | Responsibility |
|------|---------------|
| `internal/server/server.go` | `Server` struct, `New()`, `Run()` with graceful shutdown, chi router setup, `/health` |
| `internal/server/auth.go` | Basic auth middleware: validate credentials, inject user into context, 401 on failure |
| `internal/server/books.go` | JSON API handlers for books: list, detail, update, delete |
| `internal/server/tags.go` | JSON API handlers for tags, series, lookup |
| `internal/server/opds.go` | OPDS endpoint handlers: navigation + acquisition feeds, download |
| `internal/server/web.go` | Web UI handlers: render templates, serve static/cover/file, upload |
| `internal/server/render.go` | Template parsing from `embed.FS`, render helper |
| `internal/server/context.go` | Context key type for authenticated user, `UserFromContext` helper |
| `internal/server/embed.go` | `//go:embed web/templates web/static` declaration |
| `internal/opds/opds.go` | `Feed`, `Entry`, `Link`, `Author`, `Category`, `Content` structs + `MarshalXML` |
| `internal/opds/feeds.go` | Feed builder helpers: `NewNavigationFeed`, `NewAcquisitionFeed`, `BookToEntry` |
| `internal/opds/opdstest/validate.go` | `ValidateFeed`, `AssertEntry`, `AssertLink` test helpers |
| `web/templates/base.html` | Common layout (header, nav, footer, block for content) |
| `web/templates/index.html` | Book grid with covers, pagination, search bar |
| `web/templates/book.html` | Book detail page with editable form |
| `web/templates/upload.html` | File upload form |
| `web/templates/login.html` | Basic auth login page |
| `web/static/style.css` | Clean minimal CSS. No framework. |
| `main.go` (modify) | Add `serve` subcommand dispatch |
| `internal/search/fts5.go` | `FTS5Searcher` — optional upgrade behind `Searcher` interface |

---

### Task 1: HTTP Server Skeleton — Server struct, New(), Run(), chi router, /health

**Files:**
- Create: `internal/server/server.go`
- Create: `internal/server/context.go`
- Create: `internal/server/server_test.go`

**Interfaces:**
- Consumes: `config.Config` (`Config{DBPath string; Port int; StorageDir string}`), `db.Open(path) (*DB, error)`, `db.Migrate()`, `storage.New(rootDir) *Storage`
- Produces: `server.Server` struct, `server.New(cfg config.Config) (*Server, error)`, `server.Run() error`, `server.health` handler

> **Go note: `net/http` server.** Go's `http.Server` is built into the stdlib.
> You create a `*http.Server` with an `Addr` and a `Handler` (an
> `http.Handler` — anything with a `ServeHTTP(w, r)` method), then call
> `ListenAndServe()` to block. For graceful shutdown, you call
> `Shutdown(ctx)` on a separate goroutine after receiving a signal.
>
> **Go note: `signal.NotifyContext`.** This is the idiomatic way to handle
> OS signals (SIGINT/SIGTERM) in Go 1.16+. It returns a `context.Context`
> that is cancelled when the signal arrives. You `select` on `<-ctx.Done()`
> to know when to shut down.
>
> **Go note: chi router.** `chi.NewRouter()` returns a router that
> implements `http.Handler`. You register routes with methods like
> `r.Get("/path", handler)`. chi's middleware is applied via `r.Use(mw)`
> — middleware are functions that wrap handlers, similar to decorators in
> Python or middleware classes in C#/ASP.NET but as plain functions.
>
> **Go vs other languages:** In C#/ASP.NET, middleware are classes
> implementing `IMiddleware`. In Go, middleware is just a function
> `func(http.Handler) http.Handler` — you wrap the next handler and return
> a new one. No interfaces, no classes, just function composition.

- [ ] **Step 1: Write the failing test**

Create `internal/server/server_test.go`:

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	// Body will be checked more thoroughly once we implement the handler.
	// For now, just verify we get 200 + JSON.
}

func TestGracefulShutdown(t *testing.T) {
	// We can't easily test graceful shutdown in a unit test because Run()
	// blocks. Instead, we verify the server can be created and the handler
	// works. Graceful shutdown is verified manually (Ctrl-C test).
	srv := newTestServer(t)
	defer srv.Close()

	// If we got here, New() succeeded.
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// newTestServer creates a Server with temp-dir SQLite and returns an
// httptest.Server wrapping its handler.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return httptest.NewServer(srv.Handler)
}

// testConfig returns a Config pointing at temp directories.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		DBPath:     filepath.Join(t.TempDir(), "test.db"),
		Port:       0, // not used by httptest
		StorageDir: t.TempDir(),
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestHealthEndpoint -v`
Expected: FAIL — package `server` doesn't exist yet, compilation error.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/context.go`:

```go
package server

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

// contextKey is an unexported type to prevent collisions with other
// packages using context keys.
type contextKey string

const userKey contextKey = "user"

// UserFromContext extracts the authenticated user from the request context.
// Returns nil if no user is set (e.g., on unauthenticated routes).
func UserFromContext(ctx context.Context) *models.User {
	v, ok := ctx.Value(userKey).(*models.User)
	if !ok {
		return nil
	}
	return v
}
```

Create `internal/server/server.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/storage"
)

// Server is the composition root. It holds all dependencies and the chi
// router. The HTTP handler is exposed via the Handler field so tests can
// wrap it in httptest.Server.
type Server struct {
	DB      *db.DB
	Storage *storage.Storage
	Config  config.Config
	Handler http.Handler
}

// New constructs a Server by opening the database, running migrations,
// creating the storage, and building the chi router.
func New(cfg config.Config) (*Server, error) {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(); err != nil {
		database.Close()
		return nil, err
	}

	store := storage.New(cfg.StorageDir)

	s := &Server{
		DB:      database,
		Storage: store,
		Config:  cfg,
	}

	s.Handler = s.router()
	return s, nil
}

// router builds the chi router with middleware and route groups.
// This is called once during New(); the handler is stored on the Server.
func (s *Server) router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)

	// Unauthenticated routes — health checks only.
	r.Get("/health", s.health)

	// Authenticated routes — everything else.
	// Basic auth middleware will be added in Task 2.
	// For now, mount placeholder routes.
	r.Group(func(r chi.Router) {
		// Auth middleware added in Task 2.
		// Placeholder: GET / returns 501.
		r.Get("/", s.notImplemented)
	})

	return r
}

// Run starts the HTTP server with graceful shutdown on SIGINT/SIGTERM.
func (s *Server) Run() error {
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(s.Config.Port),
		Handler: s.Handler,
	}

	// signal.NotifyContext returns a context that is cancelled when the
	// specified signals arrive. This is the idiomatic pattern for
	// graceful shutdown in Go 1.16+.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start serving in a goroutine so we can wait for the signal.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("incipit serving on :%d", s.Config.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// Block until signal or serve error.
	select {
	case <-ctx.Done():
		log.Println("shutting down...")
	case err := <-errCh:
		return err
	}

	// Give 30 seconds for in-flight requests to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}

	s.DB.Close()
	log.Println("bye")
	return nil
}

// health returns a simple JSON health check. This is the only endpoint
// that does NOT require authentication (needed for k8s probes).
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// notImplemented is a placeholder for routes not yet implemented.
func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
}

// requestLogger logs each request to stderr in a compact format.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %dms",
			r.Method,
			r.URL.Path,
			ww.Status(),
			time.Since(start).Milliseconds(),
		)
	})
}
```

> **Go note: `middleware.NewWrapResponseWriter`.** chi's middleware package
> provides a response writer wrapper that captures the status code. This
> is how you log response status — Go's `http.ResponseWriter` doesn't
> expose the status code after `WriteHeader` is called, so you need a
> wrapper that records it.
>
> **Go note: error handling in goroutines.** The `errCh` pattern is the
> standard way to get errors out of a goroutine. A buffered channel of
> size 1 ensures the goroutine doesn't block if the main goroutine is
> already in the `select`.

You'll also need to add the `strconv` and `filepath` imports to the test
file. Update `internal/server/server_test.go` imports:

```go
import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/jason/incipit/internal/config"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestHealthEndpoint -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/context.go internal/server/server_test.go
git commit -m "feat(server): HTTP server skeleton with chi router, /health, graceful shutdown"
```

Questions before moving on?

---

### Task 2: Basic Auth Middleware — validate credentials, inject user, 401 on failure

**Files:**
- Create: `internal/server/auth.go`
- Modify: `internal/server/server.go` (wire middleware into router group)
- Create: `internal/server/auth_test.go`

**Interfaces:**
- Consumes: `db.GetUser(ctx, username) (*models.User, error)`, `models.User{ID, Username, PasswordHash, Role}`
- Produces: `s.basicAuth` middleware, `UserFromContext(ctx)` already exists from Task 1

> **Go note: middleware pattern.** A chi middleware is a function that takes
> an `http.Handler` and returns a new `http.Handler` that wraps it. The
> signature is `func(http.Handler) http.Handler`. Inside, you call
> `next.ServeHTTP(w, r)` to pass control to the next handler. If you want
> to reject the request (e.g., auth failure), you write the response and
> return WITHOUT calling `next.ServeHTTP`.
>
> **Go note: `bcrypt.CompareHashAndPassword`.** This function takes the
> stored hash and the plaintext input, and returns `nil` if they match. It
> handles the salt internally — you don't extract or compare salts manually.
> The hash stored in the DB is `bcrypt(MD5(password))`, and KOReader sends
> the MD5-hashed password, so the middleware compares the stored hash
> against the MD5 hash from the auth header directly.
>
> **Go vs other languages:** In C#/ASP.NET, auth middleware reads JWT
> cookies and populates `HttpContext.User`. In Go, there's no framework
> object — you inject data via `context.WithValue` and retrieve it with
> `ctx.Value(key)`. The `context.Context` travels with the request and is
> available to every handler in the chain.

- [ ] **Step 1: Write the failing test**

Create `internal/server/auth_test.go`:

```go
package server

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestBasicAuth_NoCredentials_401(t *testing.T) {
	srv := newTestServerWithUser(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Error("missing WWW-Authenticate header")
	}
}

func TestBasicAuth_ValidCredentials_200(t *testing.T) {
	srv := newTestServerWithUser(t)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// / returns 501 (notImplemented placeholder), not 401 — auth passed.
	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("got 401, expected auth to pass")
	}
}

func TestBasicAuth_WrongPassword_401(t *testing.T) {
	srv := newTestServerWithUser(t)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.SetBasicAuth("testuser", "wrongpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestBasicAuth_HealthNoAuth_200(t *testing.T) {
	srv := newTestServerWithUser(t)
	defer srv.Close()

	// /health should NOT require auth.
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health status = %d, want 200 (no auth required)", resp.StatusCode)
	}
}

func TestBasicAuth_UserInjectedIntoContext(t *testing.T) {
	srv := newTestServerWithUser(t)
	defer srv.Close()

	// We test this via a handler that echoes the user. For now, verify
	// that the placeholder handler doesn't 401 when authed — that proves
	// the middleware passed.
	req, _ := http.NewRequest("GET", srv.URL+"/", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		t.Error("auth failed but should have passed")
	}
}

// newTestServerWithUser creates a server and seeds a test user.
// The test user has password "testpass" — KOReader would MD5-hash this
// before sending, so we store bcrypt(md5("testpass")).
func newTestServerWithUser(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}

	// Seed a test user: password "testpass" → MD5 → bcrypt.
	md5hash := md5.Sum([]byte("testpass"))
	md5hex := hex.EncodeToString(md5hash[:])
	_, err = seedTestUser(srv.DB, "testuser", md5hex, "user")
	if err != nil {
		t.Fatalf("seedTestUser: %v", err)
	}

	return httptest.NewServer(srv.Handler)
}

// seedTestUser inserts a user with a bcrypt-hashed password.
// This simulates what the CLI add-user command does.
func seedTestUser(database interface{}, username, md5Password, role string) (int64, error) {
	// We use the db layer directly — but since we can't import db here
	// without creating a circular dependency in tests, we access via
	// the *db.DB pointer stored on Server. For test purposes, we use
	// the DB's raw method. This will be replaced once db exposes a
	// test helper.
	// In practice, this calls db.InsertUser which Phase 1 provides.
	return 0, context.Background().Err()
}
```

> **Important:** The `seedTestUser` function above is a placeholder. In
> practice, Phase 1's `db` package will have an `InsertUser(ctx, username,
> md5Password, role) (int64, error)` method that bcrypt-hashes internally.
> Adjust the test to call `srv.DB.InsertUser(ctx, "testuser", md5hex, "user")`
> using whatever Phase 1 actually provides. The key point: the test user's
> password is stored as `bcrypt(md5("testpass"))`, and the auth middleware
> receives `md5("testpass")` via basic auth (simulating KOReader).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestBasicAuth -v`
Expected: FAIL — `basicAuth` middleware doesn't exist, `/` returns 501
without auth check, no user seeded.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/auth.go`:

```go
package server

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// basicAuth is the middleware that validates HTTP Basic Auth credentials
// against the database. On success, injects the authenticated user into
// the request context. On failure, returns 401 with WWW-Authenticate.
//
// The password sent by KOReader is already MD5-hashed client-side. The
// stored hash is bcrypt(MD5(password)). So we compare the received
// password directly against the stored bcrypt hash — no MD5 needed here.
func (s *Server) basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok {
			unauthorized(w)
			return
		}

		user, err := s.DB.GetUser(r.Context(), username)
		if err != nil {
			// Don't distinguish "user not found" from "wrong password" —
			// both return 401 to avoid user enumeration.
			unauthorized(w)
			return
		}

		// password here is already MD5-hashed (KOReader does this).
		// user.PasswordHash is bcrypt(md5(password)).
		// bcrypt.CompareHashAndPassword handles the comparison.
		if err := bcrypt.CompareHashAndPassword(
			[]byte(user.PasswordHash),
			[]byte(password),
		); err != nil {
			unauthorized(w)
			return
		}

		// Inject user into context so handlers can access it.
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// unauthorized writes a 401 response with the WWW-Authenticate header
// prompting for Basic auth.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="incipit"`)
	w.WriteHeader(http.StatusUnauthorized)
}
```

Now modify `internal/server/server.go` to wire the auth middleware into
the authenticated route group. Replace the `r.Group` block in the
`router()` method:

```go
	// Authenticated routes — everything else.
	r.Group(func(r chi.Router) {
		r.Use(s.basicAuth)
		// Placeholder: GET / returns 501.
		r.Get("/", s.notImplemented)
	})
```

> **Go note: `r.BasicAuth()`.** This is a method on `*http.Request` (from
> `net/http`) that parses the `Authorization: Basic ...` header and returns
> `(username, password, ok)`. It handles base64 decoding for you. The
> password is whatever the client sent — in KOReader's case, that's the
> MD5-hashed password, NOT the plaintext.
>
> **Go note: `golang.org/x/crypto/bcrypt`.** This is NOT in the stdlib —
> it's in `golang.org/x/crypto`, which is the "sub-repository" of Go's
> official packages. It's effectively part of Go but lives in a separate
> module. You'll need `go get golang.org/x/crypto/bcrypt`. This is
> available because Phase 1's `add-user` command already uses it for
> password hashing.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestBasicAuth -v`
Expected: PASS (after adjusting `seedTestUser` to use the actual Phase 1
`db.InsertUser` method)

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/auth.go internal/server/auth_test.go internal/server/server.go
git commit -m "feat(server): basic auth middleware validating against db with bcrypt"
```

Questions before moving on?

---

### Task 3: JSON API — GET /api/books (paginated, filterable list)

**Files:**
- Create: `internal/server/books.go`
- Create: `internal/server/books_test.go`

**Interfaces:**
- Consumes: `db.ListBooks(ctx, ListOpts) ([]models.Book, int, error)`, `db.ListOpts{Page, PerPage, Series, Author, Tag, Query, Sort}`
- Produces: `s.handleListBooks` handler, JSON response format `{books, total, page, per_page}`

> **Go note: `chi.URLParam`.** chi's `URLParam(r, "id")` extracts path
> parameters. For query parameters, use `r.URL.Query().Get("key")`.
> Query params return strings — you parse to int with `strconv.Atoi`.
>
> **Go note: `encoding/json` response.** `json.NewEncoder(w).Encode(v)` is
> the simplest way to write JSON to an `http.ResponseWriter`. It handles
> struct-to-JSON mapping via field tags (`json:"field_name"`). Set
> `Content-Type` before encoding.
>
> **Go vs other languages:** In JS/Express, you'd `res.json(data)`. In
> Go, there's no framework sugar — you manually set headers and encode.
> This is more verbose but transparent: you see exactly what's happening.

- [ ] **Step 1: Write the failing test**

Create `internal/server/books_test.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestListBooks_Empty(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	body := authedGet(t, srv.URL+"/api/books", "testuser", "testpass")

	var resp struct {
		Books   []models.Book `json:"books"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
	if len(resp.Books) != 0 {
		t.Errorf("books = %d, want 0", len(resp.Books))
	}
}

func TestListBooks_WithData(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBooks(t, srv, 3)

	body := authedGet(t, srv.URL+"/api/books", "testuser", "testpass")

	var resp struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
		Page  int           `json:"page"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("total = %d, want 3", resp.Total)
	}
	if len(resp.Books) != 3 {
		t.Errorf("books = %d, want 3", len(resp.Books))
	}
}

func TestListBooks_Pagination(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBooks(t, srv, 5)

	// Page 1, 2 per page.
	body := authedGet(t, srv.URL+"/api/books?page=1&per_page=2", "testuser", "testpass")
	var resp struct {
		Books   []models.Book `json:"books"`
		Total   int           `json:"total"`
		Page    int           `json:"page"`
		PerPage int           `json:"per_page"`
	}
	json.Unmarshal(body, &resp)
	if resp.Total != 5 {
		t.Errorf("total = %d, want 5", resp.Total)
	}
	if len(resp.Books) != 2 {
		t.Errorf("books = %d, want 2", len(resp.Books))
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}

	// Page 3, 2 per page — should have 1 book.
	body = authedGet(t, srv.URL+"/api/books?page=3&per_page=2", "testuser", "testpass")
	json.Unmarshal(body, &resp)
	if len(resp.Books) != 1 {
		t.Errorf("page 3 books = %d, want 1", len(resp.Books))
	}
}

func TestListBooks_FilterBySeries(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "Author1", Series: "Expanse", SeriesIndex: 1})
	seedBook(t, srv, models.Book{Title: "B", Author: "Author2", Series: "Other", SeriesIndex: 1})
	seedBook(t, srv, models.Book{Title: "C", Author: "Author3", Series: "Expanse", SeriesIndex: 2})

	body := authedGet(t, srv.URL+"/api/books?series=Expanse", "testuser", "testpass")
	var resp struct {
		Books []models.Book `json:"books"`
		Total int           `json:"total"`
	}
	json.Unmarshal(body, &resp)
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2 (Expanse books)", resp.Total)
	}
}

func TestListBooks_RequiresAuth(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/books")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// authedGet makes an authenticated GET request and returns the body.
func authedGet(t *testing.T, url, user, pass string) []byte {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want 200", url, resp.StatusCode)
	}
	return mustReadBody(t, resp)
}

// mustReadBody reads the full response body, failing the test on error.
func mustReadBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// newTestServerWithData creates a server with a test user seeded.
func newTestServerWithData(t *testing.T) *httptest.Server {
	t.Helper()
	srv, err := New(testConfig(t))
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	seedTestUserDB(t, srv.DB)
	return httptest.NewServer(srv.Handler)
}

// seedTestUserDB seeds the test user directly via the db layer.
// Uses whatever InsertUser method Phase 1 provides.
func seedTestUserDB(t *testing.T, database interface{}) {
	t.Helper()
	// Phase 1 provides db.InsertUser(ctx, username, md5Password, role).
	// Adjust to match actual Phase 1 signature.
	md5hash := md5Hex("testpass")
	err := database.(*db.DB).InsertUser(context.Background(), "testuser", md5hash, "user")
	if err != nil {
		t.Fatalf("InsertUser: %v", err)
	}
}

// seedBooks inserts N test books.
func seedBooks(t *testing.T, srv *httptest.Server, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedBook(t, srv, models.Book{
			Title:  fmt.Sprintf("Book %d", i+1),
			Author: fmt.Sprintf("Author %d", i+1),
		})
	}
}

// seedBook inserts a single book via the DB layer.
func seedBook(t *testing.T, srv *httptest.Server, b models.Book) {
	t.Helper()
	_, err := srv.DB.InsertBook(context.Background(), &b)
	if err != nil {
		t.Fatalf("InsertBook: %v", err)
	}
}
```

Add the necessary imports to the test file:

```go
import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestListBooks -v`
Expected: FAIL — `handleListBooks` doesn't exist, `/api/books` returns 404
or 501.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/books.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/db"
)

// booksResponse is the JSON envelope for the book list endpoint.
type booksResponse struct {
	Books   []bookSummary `json:"books"`
	Total   int           `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"per_page"`
}

// bookSummary is the lightweight book representation for list views.
// Omits heavy fields like Description to keep responses small.
type bookSummary struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Author     string  `json:"author"`
	Series     string  `json:"series,omitempty"`
	SeriesIndex float64 `json:"series_index,omitempty"`
	Cover      string  `json:"cover,omitempty"`
}

// bookDetail is the full book representation for the detail endpoint.
type bookDetail struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	TitleSort   string    `json:"title_sort"`
	Author      string    `json:"author"`
	AuthorSort  string    `json:"author_sort"`
	Series      string    `json:"series,omitempty"`
	SeriesIndex float64   `json:"series_index,omitempty"`
	ISBN        string    `json:"isbn,omitempty"`
	Description string    `json:"description,omitempty"`
	Publisher   string    `json:"publisher,omitempty"`
	Published   string    `json:"published,omitempty"`
	Pages       int       `json:"pages,omitempty"`
	Rating      float64   `json:"rating,omitempty"`
	Cover       string    `json:"cover,omitempty"`
	FileSize    int64     `json:"file_size,omitempty"`
	Added       string    `json:"added,omitempty"`
	Tags        []tagJSON  `json:"tags,omitempty"`
}

type tagJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// handleListBooks handles GET /api/books with pagination and filtering.
//
// Query params:
//   ?page=1&per_page=20  (pagination, defaults: 1, 20)
//   ?series=The+Expanse  (filter by series name)
//   ?author=Corey        (filter by author)
//   ?tag=Science+Fiction (filter by tag)
//   ?q=search+term       (title/author search)
//   ?sort=added|title|author|series
func (s *Server) handleListBooks(w http.ResponseWriter, r *http.Request) {
	opts := parseListOpts(r)

	books, total, err := s.DB.ListBooks(r.Context(), opts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	summaries := make([]bookSummary, len(books))
	for i, b := range books {
		summaries[i] = bookSummary{
			ID:          b.ID,
			Title:       b.Title,
			Author:      b.Author,
			Series:      b.Series,
			SeriesIndex: b.SeriesIndex,
			Cover:       coverURL(b.ID),
		}
	}

	resp := booksResponse{
		Books:   summaries,
		Total:   total,
		Page:    opts.Page,
		PerPage: opts.PerPage,
	}

	writeJSON(w, http.StatusOK, resp)
}

// parseListOpts extracts pagination and filter params from the request.
func parseListOpts(r *http.Request) db.ListOpts {
	q := r.URL.Query()
	return db.ListOpts{
		Page:    atoiDefault(q.Get("page"), 1),
		PerPage: atoiDefault(q.Get("per_page"), 20),
		Series:  q.Get("series"),
		Author:  q.Get("author"),
		Tag:     q.Get("tag"),
		Query:   q.Get("q"),
		Sort:    q.Get("sort"),
	}
}

// atoiDefault parses an int, returning def on error or empty string.
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// coverURL returns the URL path for a book's cover image.
func coverURL(bookID int64) string {
	return "/covers/" + strconv.FormatInt(bookID, 10) + ".jpg"
}

// writeJSON sets headers and encodes v as JSON to w.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// (chi import is used in later handlers — keep it here.)
var _ = chi.NewRouter
```

Now wire the route into `server.go`. In the `router()` method, replace the
authenticated group:

```go
	// Authenticated routes — everything else.
	r.Group(func(r chi.Router) {
		r.Use(s.basicAuth)
		r.Get("/api/books", s.handleListBooks)
		r.Get("/", s.notImplemented) // placeholder until web UI
	})
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestListBooks -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/books.go internal/server/books_test.go internal/server/server.go
git commit -m "feat(server): GET /api/books with pagination and filtering"
```

Questions before moving on?

---

### Task 4: JSON API — GET /api/books/{id} (detail)

**Files:**
- Modify: `internal/server/books.go` (add `handleGetBook`)
- Modify: `internal/server/books_test.go` (add tests)

**Interfaces:**
- Consumes: `db.GetBook(ctx, id) (*models.Book, error)`, `db.GetTagsForBook(ctx, bookID) ([]models.Tag, error)`
- Produces: `s.handleGetBook` handler returning full `bookDetail`

> **Go note: `chi.URLParam`.** To extract a path parameter, call
> `chi.URLParam(r, "id")` — this returns the string value from the URL
> pattern `{id}`. You then parse it to `int64` with `strconv.ParseInt`.
> If parsing fails, return 400 (bad request) or 404 (not found).

- [ ] **Step 1: Write the failing test**

Add to `internal/server/books_test.go`:

```go
func TestGetBook_Exists(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{
		Title:       "Leviathan Wakes",
		Author:      "James S. A. Corey",
		Series:      "The Expanse",
		SeriesIndex: 1,
		Description: "Two hundred years after migrating into space...",
		Pages:       577,
		Rating:      4.5,
	})

	body := authedGet(t, srv.URL+"/api/books/"+strconv.FormatInt(id, 10), "testuser", "testpass")

	var b bookDetail
	if err := json.Unmarshal(body, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b.Title != "Leviathan Wakes" {
		t.Errorf("title = %q, want %q", b.Title, "Leviathan Wakes")
	}
	if b.Author != "James S. A. Corey" {
		t.Errorf("author = %q", b.Author)
	}
	if b.Series != "The Expanse" {
		t.Errorf("series = %q", b.Series)
	}
	if b.Pages != 577 {
		t.Errorf("pages = %d, want 577", b.Pages)
	}
}

func TestGetBook_NotFound_404(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/books/99999", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestGetBook_InvalidID_404(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/api/books/abc", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// chi won't match {id} to "abc" if we use a numeric regex, so this
	// should 404 (route not found). If using plain {id}, we'd get 404 from
	// the handler. Either way, 404.
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestGetBook -v`
Expected: FAIL — `handleGetBook` doesn't exist, route not registered.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/books.go`:

```go
// handleGetBook handles GET /api/books/{id} — returns full book detail
// including tags.
func (s *Server) handleGetBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	book, err := s.DB.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tags, _ := s.DB.GetTagsForBook(r.Context(), id)

	tagJSONs := make([]tagJSON, len(tags))
	for i, tag := range tags {
		tagJSONs[i] = tagJSON{ID: tag.ID, Name: tag.Name}
	}

	detail := bookDetail{
		ID:          book.ID,
		Title:       book.Title,
		TitleSort:   book.TitleSort,
		Author:      book.Author,
		AuthorSort:  book.AuthorSort,
		Series:      book.Series,
		SeriesIndex: book.SeriesIndex,
		ISBN:        book.ISBN,
		Description: book.Description,
		Publisher:   book.Publisher,
		Published:   book.Published,
		Pages:       book.Pages,
		Rating:      book.Rating,
		Cover:       coverURL(book.ID),
		FileSize:    book.FileSize,
		Added:       book.Added,
		Tags:        tagJSONs,
	}

	writeJSON(w, http.StatusOK, detail)
}
```

Register the route in `server.go`. In the `router()` method's authenticated
group, add:

```go
		r.Get("/api/books/{id}", s.handleGetBook)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestGetBook -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/books.go internal/server/books_test.go internal/server/server.go
git commit -m "feat(server): GET /api/books/{id} book detail with tags"
```

Questions before moving on?

---

### Task 5: JSON API — PUT /api/books/{id}, DELETE /api/books/{id}

**Files:**
- Modify: `internal/server/books.go` (add `handleUpdateBook`, `handleDeleteBook`)
- Modify: `internal/server/books_test.go` (add tests)

**Interfaces:**
- Consumes: `db.UpdateBook(ctx, *models.Book)`, `db.DeleteBook(ctx, id)`, `storage.DeleteBookFile(id)`, `storage.DeleteCover(id)`
- Produces: `s.handleUpdateBook`, `s.handleDeleteBook` handlers

> **Go note: reading request body.** `json.NewDecoder(r.Body).Decode(&v)`
> reads and parses JSON from the request body in one step. Always check
> the error — malformed JSON returns an error, not a panic. Close `r.Body`
> is handled automatically by `net/http` (Go's server reads to EOF and
> closes), but it's good practice to `defer r.Body.Close()` in non-server
> contexts.
>
> **Go note: `http.NoBody` and DELETE.** A DELETE request typically has no
> body. Go's `http.NewRequest("DELETE", url, nil)` passes `nil` for the
> body, which is fine. For requests with a body, pass `bytes.NewReader([]byte)`.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/books_test.go`:

```go
func TestUpdateBook(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{
		Title:  "Original Title",
		Author: "Original Author",
	})

	updateBody := `{"title":"Updated Title","author":"Updated Author","series":"New Series","series_index":3}`
	req, _ := http.NewRequest("PUT", srv.URL+"/api/books/"+strconv.FormatInt(id, 10), strings.NewReader(updateBody))
	req.SetBasicAuth("testuser", "testpass")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Verify via GET.
	body := authedGet(t, srv.URL+"/api/books/"+strconv.FormatInt(id, 10), "testuser", "testpass")
	var b bookDetail
	json.Unmarshal(body, &b)
	if b.Title != "Updated Title" {
		t.Errorf("title = %q, want 'Updated Title'", b.Title)
	}
	if b.Author != "Updated Author" {
		t.Errorf("author = %q", b.Author)
	}
	if b.Series != "New Series" {
		t.Errorf("series = %q", b.Series)
	}
}

func TestDeleteBook(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{
		Title:  "To Delete",
		Author: "Author",
	})

	// DELETE
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/books/"+strconv.FormatInt(id, 10), nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}

	// Verify gone via GET → 404.
	req, _ = http.NewRequest("GET", srv.URL+"/api/books/"+strconv.FormatInt(id, 10), nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete: status = %d, want 404", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestUpdateBook -v`
Run: `go test ./internal/server/ -run TestDeleteBook -v`
Expected: FAIL — handlers don't exist, routes not registered.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/books.go`:

```go
// bookUpdate is the request body for PUT /api/books/{id}.
// Only fields present in the JSON are updated — omitted fields are
// zero-valued. In a future iteration we could use *string pointers for
// optional fields, but for now the caller sends the full object.
type bookUpdate struct {
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Series      string  `json:"series"`
	SeriesIndex float64 `json:"series_index"`
	Description string  `json:"description"`
	Rating      float64 `json:"rating"`
	Tags        []string `json:"tags"`
}

// handleUpdateBook handles PUT /api/books/{id} — updates book metadata.
func (s *Server) handleUpdateBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var update bookUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Fetch existing book to preserve fields not in the update.
	book, err := s.DB.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	book.Title = update.Title
	book.Author = update.Author
	book.Series = update.Series
	book.SeriesIndex = update.SeriesIndex
	book.Description = update.Description
	book.Rating = update.Rating

	if err := s.DB.UpdateBook(r.Context(), book); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if len(update.Tags) > 0 {
		if err := s.DB.AddTagsToBook(r.Context(), id, update.Tags); err != nil {
			// Non-fatal — book updated, tags failed.
			// In production, log this. For now, proceed.
		}
	}

	// Return the updated book.
	s.handleGetBook(w, r)
}

// handleDeleteBook handles DELETE /api/books/{id} — deletes book record,
// file, and cover.
func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Delete DB record (cascades to book_tags, reading_progress).
	if err := s.DB.DeleteBook(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}

	// Best-effort file/cover cleanup — non-fatal if they don't exist.
	_ = s.Storage.DeleteBookFile(id)
	_ = s.Storage.DeleteCover(id)

	w.WriteHeader(http.StatusNoContent)
}
```

Add the `strings` import to the test file (if not already present).

Register routes in `server.go`. In the authenticated group:

```go
		r.Put("/api/books/{id}", s.handleUpdateBook)
		r.Delete("/api/books/{id}", s.handleDeleteBook)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestUpdateBook -v`
Run: `go test ./internal/server/ -run TestDeleteBook -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/books.go internal/server/books_test.go internal/server/server.go
git commit -m "feat(server): PUT/DELETE /api/books/{id} update and delete"
```

Questions before moving on?

---

### Task 6: JSON API — GET /api/tags, GET /api/series, GET /api/lookup

**Files:**
- Create: `internal/server/tags.go`
- Create: `internal/server/tags_test.go`

**Interfaces:**
- Consumes: `db.ListTags(ctx) ([]models.Tag, error)`, `db.ListSeries(ctx) ([]db.SeriesInfo, error)`, `lookup.Lookup(ctx, isbn, title, author) (*models.LookupResult, error)`
- Produces: `s.handleListTags`, `s.handleListSeries`, `s.handleLookup` handlers

> **Go note: content negotiation.** When a client sends `Accept: application/json`,
> you could check `r.Header.Get("Accept")` and respond accordingly. For
> simplicity, the `/api/*` routes always return JSON — no negotiation
> needed. The OPDS routes (`/opds/*`) always return XML. The web UI routes
> always return HTML. Each route group has a fixed content type.

- [ ] **Step 1: Write the failing test**

Create `internal/server/tags_test.go`:

```go
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestListTags(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	// Seed books with tags.
	id1 := seedBook(t, srv, models.Book{Title: "A", Author: "Auth"})
	id2 := seedBook(t, srv, models.Book{Title: "B", Author: "Auth"})

	_ = srv.DB.AddTagsToBook(context.Background(), id1, []string{"Fiction", "Sci-Fi"})
	_ = srv.DB.AddTagsToBook(context.Background(), id2, []string{"Fiction"})

	body := authedGet(t, srv.URL+"/api/tags", "testuser", "testpass")

	var tags []models.Tag
	if err := json.Unmarshal(body, &tags); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tags) < 2 {
		t.Errorf("tags = %d, want >= 2", len(tags))
	}
}

func TestListSeries(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "Auth", Series: "Expanse", SeriesIndex: 1})
	seedBook(t, srv, models.Book{Title: "B", Author: "Auth", Series: "Expanse", SeriesIndex: 2})
	seedBook(t, srv, models.Book{Title: "C", Author: "Auth", Series: "Foundation", SeriesIndex: 1})

	body := authedGet(t, srv.URL+"/api/series", "testuser", "testpass")

	var series []struct {
		Name      string `json:"name"`
		BookCount int    `json:"book_count"`
	}
	if err := json.Unmarshal(body, &series); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d, want 2", len(series))
	}

	// Find Expanse.
	var found bool
	for _, s := range series {
		if s.Name == "Expanse" {
			if s.BookCount != 2 {
				t.Errorf("Expanse count = %d, want 2", s.BookCount)
			}
			found = true
		}
	}
	if !found {
		t.Error("Expanse series not found")
	}
}

func TestLookupByISBN(t *testing.T) {
	// This test uses a mock lookup — we can't hit the real network.
	// For now, test that the endpoint exists and returns JSON.
	// Full integration test with httptest mock is in Task 13.
	srv := newTestServerWithData(t)
	defer srv.Close()

	// Without a mock, this will fail to reach the network.
	// We test the route exists and returns 200 or a graceful error.
	req, _ := http.NewRequest("GET", srv.URL+"/api/lookup?isbn=9780316129084", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skip("network not available, skipping lookup test")
	}
	defer resp.Body.Close()
	// Accept either 200 (found) or 502 (lookup failed) — both prove
	// the route works.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 200 or 502", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestListTags -v`
Run: `go test ./internal/server/ -run TestListSeries -v`
Expected: FAIL — handlers don't exist, routes not registered.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/tags.go`:

```go
package server

import (
	"net/http"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
)

// handleListTags handles GET /api/tags — returns all tags.
func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.DB.ListTags(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if tags == nil {
		tags = []models.Tag{}
	}
	writeJSON(w, http.StatusOK, tags)
}

// seriesResponse is the JSON representation of a series with book count.
type seriesResponse struct {
	Name      string `json:"name"`
	BookCount int    `json:"book_count"`
}

// handleListSeries handles GET /api/series — returns all series with
// book counts.
func (s *Server) handleListSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.DB.ListSeries(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := make([]seriesResponse, len(series))
	for i, s := range series {
		resp[i] = seriesResponse{
			Name:      s.Name,
			BookCount: s.BookCount,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLookup handles GET /api/lookup — queries external metadata APIs.
// Query params: ?isbn=X or ?title=T&author=A
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	isbn := q.Get("isbn")
	title := q.Get("title")
	author := q.Get("author")

	if isbn == "" && title == "" {
		http.Error(w, `{"error":"isbn or title required"}`, http.StatusBadRequest)
		return
	}

	result, err := lookup.Lookup(r.Context(), isbn, title, author)
	if err != nil {
		// Lookup failed — return 502 (bad gateway) to indicate upstream
		// failure, not server error. The book can still be added with
		// EPUB-only metadata.
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "lookup failed: " + err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Ensure db import is used (for SeriesInfo reference in comments).
var _ db.SeriesInfo
```

Register routes in `server.go`. In the authenticated group:

```go
		r.Get("/api/tags", s.handleListTags)
		r.Get("/api/series", s.handleListSeries)
		r.Get("/api/lookup", s.handleLookup)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run "TestListTags|TestListSeries" -v`
Expected: PASS (TestLookupByISBN may skip if no network)

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/tags.go internal/server/tags_test.go internal/server/server.go
git commit -m "feat(server): GET /api/tags, /api/series, /api/lookup"
```

Questions before moving on?

---

### Task 7: internal/opds — Feed/Entry structs, MarshalXML, opdstest validator

**Files:**
- Create: `internal/opds/opds.go`
- Create: `internal/opds/feeds.go`
- Create: `internal/opds/opdstest/validate.go`
- Create: `internal/opds/opds_test.go`

**Interfaces:**
- Consumes: `models.Book` struct fields
- Produces: `opds.Feed`, `opds.Entry`, `opds.Link`, `opds.Author`, `opds.Category`, `opds.Content`, `opds.MarshalXML`, `opds.NewNavigationFeed`, `opds.NewAcquisitionFeed`, `opds.BookToEntry`, `opdstest.ValidateFeed`, `opdstest.AssertEntry`, `opdstest.AssertLink`

> **Go note: `encoding/xml` struct tags.** Go's XML marshaling uses struct
> tags like `xml:"element_name"` and `xml:"attr,attr"` for attributes.
> The `xml:"-"` tag excludes a field. For namespace-aware XML, you set the
> `XMLName` field with a `xml.Name` that includes the namespace URI.
>
> **Go note: `xml.MarshalIndent`.** For human-readable XML, use
> `xml.MarshalIndent(v, "", "  ")`. For production, `xml.Marshal(v)`
> produces compact XML without indentation.
>
> **Go vs other languages:** In Python, you'd use `lxml` or `xml.etree`
> with explicit element construction. In Go, you define a struct with
> tags and the marshaler does the work — similar to `encoding/json` tags
> you already know. The key difference: Go's XML marshaling is
> declarative (struct tags) not imperative (builder API).

- [ ] **Step 1: Write the failing test**

Create `internal/opds/opds_test.go`:

```go
package opds

import (
	"encoding/xml"
	"strings"
	"testing"

	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/opds/opdstest"
)

func TestNavigationFeed_MarshalXML(t *testing.T) {
	feed := NewNavigationFeed("urn:incipit:root", "Incipit Library", "/opds")
	feed.AddNavEntry("Newest Books", "/opds/newest")
	feed.AddNavEntry("By Author", "/opds/byauthor")

	data, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	xmlStr := string(data)
	if !strings.Contains(xmlStr, `<feed`) {
		t.Error("missing <feed> root element")
	}
	if !strings.Contains(xmlStr, "Incipit Library") {
		t.Error("missing title")
	}
	if !strings.Contains(xmlStr, "urn:incipit:root") {
		t.Error("missing id")
	}

	// Use the validator.
	opdstest.ValidateFeed(t, data)
}

func TestAcquisitionFeed_MarshalXML(t *testing.T) {
	book := models.Book{
		ID:          1,
		Title:       "Leviathan Wakes",
		Author:      "James S. A. Corey",
		Series:      "The Expanse",
		SeriesIndex: 1,
		Description: "Two hundred years after migrating into space...",
	}

	feed := NewAcquisitionFeed("urn:incipit:newest", "Newest Books", "/opds/newest")
	feed.AddEntry(BookToEntry(book))

	data, err := xml.MarshalIndent(feed, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	xmlStr := string(data)
	if !strings.Contains(xmlStr, "Leviathan Wakes") {
		t.Error("missing book title")
	}
	if !strings.Contains(xmlStr, "application/epub+zip") {
		t.Error("missing acquisition link type")
	}
	if !strings.Contains(xmlStr, "/opds/book/1/download") {
		t.Error("missing download link")
	}
	if !strings.Contains(xmlStr, "/covers/1.jpg") {
		t.Error("missing cover image link")
	}

	opdstest.ValidateFeed(t, data)
}

func TestAcquisitionFeed_HasCorrectContentType(t *testing.T) {
	feed := NewAcquisitionFeed("urn:incipit:test", "Test", "/opds/test")
	if feed.SelfLink.Type != ContentAcquisition {
		t.Errorf("self link type = %q, want %q", feed.SelfLink.Type, ContentAcquisition)
	}
}

func TestNavigationFeed_HasCorrectContentType(t *testing.T) {
	feed := NewNavigationFeed("urn:incipit:test", "Test", "/opds/test")
	if feed.SelfLink.Type != ContentNavigation {
		t.Errorf("self link type = %q, want %q", feed.SelfLink.Type, ContentNavigation)
	}
}

func TestBookToEntry_WithSeries(t *testing.T) {
	book := models.Book{
		ID:     42,
		Title:  "Test Book",
		Author: "Test Author",
		Series: "Test Series",
	}

	entry := BookToEntry(book)

	if entry.Title != "Test Book" {
		t.Errorf("title = %q", entry.Title)
	}
	if entry.ID != "urn:incipit:book:42" {
		t.Errorf("id = %q, want urn:incipit:book:42", entry.ID)
	}

	// Should have a series category.
	var foundSeries bool
	for _, cat := range entry.Categories {
		if cat.Label == "series" && cat.Term == "Test Series" {
			foundSeries = true
		}
	}
	if !foundSeries {
		t.Error("missing series category")
	}

	// Should have acquisition + image links.
	var foundAcq, foundImg bool
	for _, link := range entry.Links {
		if link.Rel == RelAcquisition {
			foundAcq = true
		}
		if link.Rel == RelImage {
			foundImg = true
		}
	}
	if !foundAcq {
		t.Error("missing acquisition link")
	}
	if !foundImg {
		t.Error("missing image link")
	}
}
```

Create `internal/opds/opdstest/validate.go`:

```go
package opdstest

import (
	"encoding/xml"
	"testing"

	"github.com/jason/incipit/internal/opds"
)

// ValidateFeed unmarshals feed XML and asserts basic OPDS structure.
// Call this in tests to catch XML format bugs.
func ValidateFeed(t *testing.T, data []byte) {
	t.Helper()

	var feed opds.Feed
	if err := xml.Unmarshal(data, &feed); err != nil {
		t.Fatalf("invalid XML: %v\nXML:\n%s", err, string(data))
	}

	if feed.ID == "" {
		t.Error("feed missing <id>")
	}
	if feed.Title == "" {
		t.Error("feed missing <title>")
	}
	if feed.Updated == "" {
		t.Error("feed missing <updated>")
	}

	// Self link should always be present.
	var hasSelf bool
	for _, link := range feed.Links {
		if link.Rel == "self" {
			hasSelf = true
			if link.Href == "" {
				t.Error("self link missing href")
			}
			if link.Type == "" {
				t.Error("self link missing type")
			}
		}
	}
	if !hasSelf {
		t.Error("feed missing <link rel=\"self\">")
	}
}

// AssertEntry validates a single OPDS entry has required fields.
func AssertEntry(t *testing.T, e opds.Entry) {
	t.Helper()
	if e.ID == "" {
		t.Error("entry missing <id>")
	}
	if e.Title == "" {
		t.Error("entry missing <title>")
	}
}

// AssertLink checks that a link with the given rel exists in the list.
func AssertLink(t *testing.T, links []opds.Link, rel string) {
	t.Helper()
	for _, link := range links {
		if link.Rel == rel {
			return
		}
	}
	t.Errorf("missing <link rel=%q>", rel)
}

// AssertLinkType checks that a link with the given rel AND type exists.
func AssertLinkType(t *testing.T, links []opds.Link, rel, typ string) {
	t.Helper()
	for _, link := range links {
		if link.Rel == rel && link.Type == typ {
			return
		}
	}
	t.Errorf("missing <link rel=%q type=%q>", rel, typ)
}

// AssertEntryCount checks the feed has the expected number of entries.
func AssertEntryCount(t *testing.T, feed *opds.Feed, want int) {
	t.Helper()
	if len(feed.Entries) != want {
		t.Errorf("entries = %d, want %d", len(feed.Entries), want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/opds/ -v`
Expected: FAIL — package `opds` doesn't exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/opds/opds.go`:

```go
package opds

import (
	"encoding/xml"
	"time"
)

// Content type constants for OPDS feeds.
const (
	ContentNavigation = "application/atom+xml; profile=opds-catalog; kind=navigation"
	ContentAcquisition = "application/atom+xml; profile=opds-catalog; kind=acquisition"
	ContentTypeAtom    = "application/atom+xml; profile=opds-catalog"
)

// Link relation constants.
const (
	RelSelf        = "self"
	RelStart       = "start"
	RelNext        = "next"
	RelSubsection  = "subsection"
	RelSearch       = "search"
	RelImage       = "http://opds-spec.org/image"
	RelAcquisition = "http://opds-spec.org/acquisition"
)

// Feed is the root OPDS Atom feed element.
type Feed struct {
	XMLName xml.Name `xml:"http://www.w3.org/2005/Atom feed"`
	ID      string   `xml:"id"`
	Title   string   `xml:"title"`
	Updated string   `xml:"updated"`
	Author  *Author  `xml:"author,omitempty"`
	Links   []Link   `xml:"link"`
	Entries []Entry  `xml:"entry"`
}

// Link is an Atom <link> element with rel, href, and type attributes.
type Link struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
	Type string `xml:"type,attr"`
}

// Author is an Atom <author> element.
type Author struct {
	Name string `xml:"name"`
	URI  string `xml:"uri,omitempty"`
}

// Entry is an Atom <entry> element — represents a book or a navigation
// node in an OPDS feed.
type Entry struct {
	ID         string     `xml:"id"`
	Title      string     `xml:"title"`
	Author     *Author    `xml:"author,omitempty"`
	Updated    string     `xml:"updated,omitempty"`
	Published  string     `xml:"published,omitempty"`
	Categories []Category `xml:"category,omitempty"`
	Content    *Content   `xml:"content,omitempty"`
	Links      []Link     `xml:"link"`
}

// Category is an Atom <category> element with term and label attributes.
type Category struct {
	Term  string `xml:"term,attr"`
	Label string `xml:"label,attr"`
}

// Content is an Atom <content> element with a type attribute and text body.
type Content struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

// SelfLink is a convenience accessor for the feed's self link.
// This field is not marshaled — it's used by builders and tests.
type feedBuilder struct {
	SelfLink Link
}
```

Create `internal/opds/feeds.go`:

```go
package opds

import (
	"fmt"
	"time"

	"github.com/jason/incipit/internal/models"
)

// NewNavigationFeed creates a navigation feed with self and start links.
// Navigation feeds list categories (authors, series, tags) — no download
// links.
func NewNavigationFeed(id, title, selfHref string) *Feed {
	now := time.Now().UTC().Format(time.RFC3339)
	return &Feed{
		ID:      id,
		Title:   title,
		Updated: now,
		Author: &Author{
			Name: "Incipit",
		},
		Links: []Link{
			{Rel: RelSelf, Href: selfHref, Type: ContentNavigation},
			{Rel: RelStart, Href: "/opds", Type: ContentNavigation},
		},
	}
}

// NewAcquisitionFeed creates an acquisition feed with self and start links.
// Acquisition feeds list books with download links.
func NewAcquisitionFeed(id, title, selfHref string) *Feed {
	now := time.Now().UTC().Format(time.RFC3339)
	return &Feed{
		ID:      id,
		Title:   title,
		Updated: now,
		Author: &Author{
			Name: "Incipit",
		},
		Links: []Link{
			{Rel: RelSelf, Href: selfHref, Type: ContentAcquisition},
			{Rel: RelStart, Href: "/opds", Type: ContentNavigation},
		},
	}
}

// AddNextLink adds a <link rel="next"> for pagination.
func (f *Feed) AddNextLink(href string) {
	f.Links = append(f.Links, Link{Rel: RelNext, Href: href, Type: ContentAcquisition})
}

// AddNavEntry adds a navigation entry (subsection link) to a navigation feed.
func (f *Feed) AddNavEntry(title, href string) {
	f.Entries = append(f.Entries, Entry{
		Title: title,
		Links: []Link{
			{Rel: RelSubsection, Href: href, Type: ContentNavigation},
		},
	})
}

// AddEntry adds a book entry to the feed.
func (f *Feed) AddEntry(entry Entry) {
	f.Entries = append(f.Entries, entry)
}

// BookToEntry converts a models.Book to an OPDS Entry with acquisition and
// image links.
func BookToEntry(book models.Book) Entry {
	entry := Entry{
		ID:       fmt.Sprintf("urn:incipit:book:%d", book.ID),
		Title:    book.Title,
		Updated:  book.Updated,
		Published: book.Added,
		Author: &Author{
			Name: book.Author,
		},
		Links: []Link{
			{
				Rel:  RelImage,
				Href: fmt.Sprintf("/covers/%d.jpg", book.ID),
				Type: "image/jpeg",
			},
			{
				Rel:  RelAcquisition,
				Href: fmt.Sprintf("/opds/book/%d/download", book.ID),
				Type: "application/epub+zip",
			},
		},
	}

	if book.Description != "" {
		entry.Content = &Content{
			Type: "text",
			Body: book.Description,
		}
	}

	if book.Series != "" {
		entry.Categories = append(entry.Categories, Category{
			Term:  book.Series,
			Label: "series",
		})
	}

	return entry
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/opds/ -v`
Expected: PASS

Run: `go vet ./internal/opds/...`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/opds/opds.go internal/opds/feeds.go internal/opds/opds_test.go internal/opds/opdstest/validate.go
git commit -m "feat(opds): Feed/Entry structs, MarshalXML, opdstest validator"
```

Questions before moving on?

---

### Task 8: OPDS Navigation Feeds — root, byauthor, byseries, bytag

**Files:**
- Create: `internal/server/opds.go`
- Create: `internal/server/opds_test.go`

**Interfaces:**
- Consumes: `opds.NewNavigationFeed(id, title, href)`, `feed.AddNavEntry(title, href)`, `opds.ContentNavigation`, `db.ListSeries(ctx)`, `db.ListTags(ctx)`
- Produces: `s.handleOPDSRoot`, `s.handleOPDSByAuthor`, `s.handleOPDSBySeries`, `s.handleOPDSByTag` handlers

> **Go note: `xml.Marshal` vs `xml.NewEncoder`.** For writing directly to an
> `http.ResponseWriter`, use `xml.NewEncoder(w).Encode(feed)` — it streams
> the output instead of buffering the whole XML in memory. For tests where
> you need the bytes, use `xml.Marshal(feed)`.
>
> **Go note: setting Content-Type.** Always set `Content-Type` BEFORE
> writing the body. Once you call `w.Write()` or the encoder writes, the
> headers are flushed. Set it explicitly for every OPDS endpoint — don't
> rely on Go's sniffing, which guesses `text/xml` instead of the OPDS
> content type KOReader expects.

- [ ] **Step 1: Write the failing test**

Create `internal/server/opds_test.go`:

```go
package server

import (
	"encoding/xml"
	"net/http"
	"testing"

	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/opds"
	"github.com/jason/incipit/internal/opds/opdstest"
)

func TestOPDSRoot(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	body := authedGet(t, srv.URL+"/opds", "testuser", "testpass")

	var feed opds.Feed
	if err := xml.Unmarshal(body, &feed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if feed.ID != "urn:incipit:root" {
		t.Errorf("id = %q, want urn:incipit:root", feed.ID)
	}

	// Root should link to: newest, byauthor, byseries, bytag, search.
	wantEntries := map[string]bool{
		"Newest Books": false,
		"By Author":    false,
		"By Series":    false,
		"By Tag":       false,
		"Search":       false,
	}
	for _, e := range feed.Entries {
		if _, ok := wantEntries[e.Title]; ok {
			wantEntries[e.Title] = true
		}
	}
	for title, found := range wantEntries {
		if !found {
			t.Errorf("missing nav entry %q", title)
		}
	}

	// Validate content type on the HTTP response.
	resp := authedGetRaw(t, srv.URL+"/opds", "testuser", "testpass")
	if resp.Header.Get("Content-Type") != opds.ContentNavigation {
		t.Errorf("Content-Type = %q, want %q",
			resp.Header.Get("Content-Type"), opds.ContentNavigation)
	}
}

func TestOPDSByAuthor(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "Corey"})
	seedBook(t, srv, models.Book{Title: "B", Author: "Scalzi"})
	seedBook(t, srv, models.Book{Title: "C", Author: "Corey"})

	body := authedGet(t, srv.URL+"/opds/byauthor", "testuser", "testpass")

	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	// Should have 2 entries (Corey, Scalzi).
	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}
}

func TestOPDSBySeries(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "X", Series: "Expanse"})
	seedBook(t, srv, models.Book{Title: "B", Author: "X", Series: "Foundation"})

	body := authedGet(t, srv.URL+"/opds/byseries", "testuser", "testpass")

	var feed opds.Feed
	xml.Unmarshal(body, &feed)
	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}
}

func TestOPDSByTag(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id1 := seedBook(t, srv, models.Book{Title: "A", Author: "X"})
	_ = srv.DB.AddTagsToBook(testContext(), id1, []string{"Fiction", "Sci-Fi"})

	body := authedGet(t, srv.URL+"/opds/bytag", "testuser", "testpass")

	var feed opds.Feed
	xml.Unmarshal(body, &feed)
	if len(feed.Entries) < 2 {
		t.Errorf("entries = %d, want >= 2", len(feed.Entries))
	}
}

// authedGetRaw returns the full *http.Response for inspection.
func authedGetRaw(t *testing.T, url, user, pass string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(user, pass)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func testContext() context.Context {
	return context.Background()
}

// Ensure opdstest import is used.
var _ = opdstest.ValidateFeed
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestOPDS -v`
Expected: FAIL — OPDS handlers don't exist, routes not registered.

- [ ] **Step 3: Write minimal implementation**

Create `internal/server/opds.go`:

```go
package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/opds"
)

// writeOPDS sets the OPDS content type and encodes the feed as XML.
func writeOPDS(w http.ResponseWriter, contentType string, feed *opds.Feed) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_ = xml.NewEncoder(w).Encode(feed)
}

// handleOPDSRoot handles GET /opds — root navigation feed.
func (s *Server) handleOPDSRoot(w http.ResponseWriter, r *http.Request) {
	feed := opds.NewNavigationFeed("urn:incipit:root", "Incipit Library", "/opds")
	feed.AddNavEntry("Newest Books", "/opds/newest")
	feed.AddNavEntry("By Author", "/opds/byauthor")
	feed.AddNavEntry("By Series", "/opds/byseries")
	feed.AddNavEntry("By Tag", "/opds/bytag")
	feed.AddNavEntry("Search", "/opds/search")
	writeOPDS(w, opds.ContentNavigation, feed)
}

// handleOPDSByAuthor handles GET /opds/byauthor — navigation feed listing
// all authors.
func (s *Server) handleOPDSByAuthor(w http.ResponseWriter, r *http.Request) {
	authors, err := s.DB.ListAuthors(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feed := opds.NewNavigationFeed("urn:incipit:byauthor", "Books by Author", "/opds/byauthor")
	for _, author := range authors {
		href := "/opds/byauthor/" + url.PathEscape(author)
		feed.AddNavEntry(author, href)
	}
	writeOPDS(w, opds.ContentNavigation, feed)
}

// handleOPDSBySeries handles GET /opds/byseries — navigation feed listing
// all series with book counts.
func (s *Server) handleOPDSBySeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.DB.ListSeries(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feed := opds.NewNavigationFeed("urn:incipit:byseries", "Books by Series", "/opds/byseries")
	for _, s := range series {
		title := s.Name + " (" + strconv.Itoa(s.BookCount) + ")"
		href := "/opds/byseries/" + url.PathEscape(s.Name)
		feed.AddNavEntry(title, href)
	}
	writeOPDS(w, opds.ContentNavigation, feed)
}

// handleOPDSByTag handles GET /opds/bytag — navigation feed listing all tags.
func (s *Server) handleOPDSByTag(w http.ResponseWriter, r *http.Request) {
	tags, err := s.DB.ListTags(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feed := opds.NewNavigationFeed("urn:incipit:bytag", "Books by Tag", "/opds/bytag")
	for _, tag := range tags {
		href := "/opds/bytag/" + url.PathEscape(tag.Name)
		feed.AddNavEntry(tag.Name, href)
	}
	writeOPDS(w, opds.ContentNavigation, feed)
}
```

> **Note:** `s.DB.ListAuthors(ctx) ([]string, error)` is needed. If Phase 1
> doesn't provide it, add a method to `internal/db` that does
> `SELECT DISTINCT author FROM books ORDER BY author_sort`. This is a
> simple addition — include it in the db package if missing.

Register routes in `server.go`. In the authenticated group:

```go
		r.Get("/opds", s.handleOPDSRoot)
		r.Get("/opds/byauthor", s.handleOPDSByAuthor)
		r.Get("/opds/byseries", s.handleOPDSBySeries)
		r.Get("/opds/bytag", s.handleOPDSByTag)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestOPDS -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/opds.go internal/server/opds_test.go internal/server/server.go
git commit -m "feat(server): OPDS navigation feeds — root, byauthor, byseries, bytag"
```

Questions before moving on?

---

### Task 9: OPDS Acquisition Feeds — newest, byauthor/{author}, byseries/{series}, bytag/{tag}, search

**Files:**
- Modify: `internal/server/opds.go` (add acquisition handlers)
- Modify: `internal/server/opds_test.go` (add tests)

**Interfaces:**
- Consumes: `opds.NewAcquisitionFeed(id, title, href)`, `feed.AddEntry(opds.BookToEntry(book))`, `feed.AddNextLink(href)`, `db.ListBooks(ctx, ListOpts)`, `db.ListBooksByAuthor(ctx, author, limit, offset)`, `db.ListBooksBySeries(ctx, series, limit, offset)`, `db.ListBooksByTag(ctx, tag, limit, offset)`, `search.Searcher.Search(ctx, q, opts)`
- Produces: `s.handleOPDSNewest`, `s.handleOPDSByAuthorBooks`, `s.handleOPDSBySeriesBooks`, `s.handleOPDSByTagBooks`, `s.handleOPDSSearch` handlers

> **Go note: pagination in OPDS.** OPDS uses `<link rel="next">` for
> pagination. The convention: if there are more entries beyond the current
> page, include a next link pointing to `?page=N+1`. The client follows
> next links automatically. 50 entries per page is the spec's default.
>
> **Go note: `url.PathEscape`.** When embedding user data (author names,
> series names, tag names) in URL paths, you must escape them. A series
> like "The Expanse" becomes "The%20Expanse" in the URL. Use
> `url.PathEscape(s)` for path segments. To decode, use `url.PathUnescape(s)`.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/opds_test.go`:

```go
func TestOPDSNewest(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	for i := 0; i < 3; i++ {
		seedBook(t, srv, models.Book{
			Title:  fmt.Sprintf("Book %d", i+1),
			Author: fmt.Sprintf("Author %d", i+1),
		})
	}

	body := authedGet(t, srv.URL+"/opds/newest", "testuser", "testpass")

	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 3 {
		t.Errorf("entries = %d, want 3", len(feed.Entries))
	}

	// Each entry should have an acquisition link.
	for _, e := range feed.Entries {
		opdstest.AssertLink(t, e.Links, opds.RelAcquisition)
	}
}

func TestOPDSNewest_Pagination(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	for i := 0; i < 60; i++ {
		seedBook(t, srv, models.Book{
			Title:  fmt.Sprintf("Book %d", i+1),
			Author: "Author",
		})
	}

	body := authedGet(t, srv.URL+"/opds/newest?page=1", "testuser", "testpass")
	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 50 {
		t.Errorf("page 1 entries = %d, want 50", len(feed.Entries))
	}

	// Should have a next link.
	opdstest.AssertLink(t, feed.Links, opds.RelNext)
}

func TestOPDSByAuthorBooks(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "Corey"})
	seedBook(t, srv, models.Book{Title: "B", Author: "Corey"})
	seedBook(t, srv, models.Book{Title: "C", Author: "Scalzi"})

	body := authedGet(t, srv.URL+"/opds/byauthor/Corey", "testuser", "testpass")
	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2 (Corey books)", len(feed.Entries))
	}
}

func TestOPDSBySeriesBooks(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "A", Author: "X", Series: "Expanse", SeriesIndex: 2})
	seedBook(t, srv, models.Book{Title: "B", Author: "X", Series: "Expanse", SeriesIndex: 1})

	body := authedGet(t, srv.URL+"/opds/byseries/Expanse", "testuser", "testpass")
	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}

	// Should be ordered by series_index: B (1) before A (2).
	if feed.Entries[0].Title != "B" {
		t.Errorf("first entry = %q, want 'B' (series_index 1)", feed.Entries[0].Title)
	}
}

func TestOPDSByTagBooks(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id1 := seedBook(t, srv, models.Book{Title: "A", Author: "X"})
	id2 := seedBook(t, srv, models.Book{Title: "B", Author: "X"})
	_ = srv.DB.AddTagsToBook(testContext(), id1, []string{"Fiction"})
	_ = srv.DB.AddTagsToBook(testContext(), id2, []string{"Fiction"})

	body := authedGet(t, srv.URL+"/opds/bytag/Fiction", "testuser", "testpass")
	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(feed.Entries))
	}
}

func TestOPDSSearch(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "Leviathan Wakes", Author: "Corey"})
	seedBook(t, srv, models.Book{Title: "Caliban's War", Author: "Corey"})

	body := authedGet(t, srv.URL+"/opds/search?q=Leviathan", "testuser", "testpass")
	var feed opds.Feed
	xml.Unmarshal(body, &feed)

	if len(feed.Entries) != 1 {
		t.Errorf("entries = %d, want 1 (Leviathan Wakes)", len(feed.Entries))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run "TestOPDSNewest|TestOPDSByAuthorBooks|TestOPDSSearch" -v`
Expected: FAIL — acquisition handlers don't exist, routes not registered.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/opds.go`:

```go
const opdsPageSize = 50

// handleOPDSNewest handles GET /opds/newest — acquisition feed of newest
// books, 50 per page.
func (s *Server) handleOPDSNewest(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	offset := (page - 1) * opdsPageSize

	opts := db.ListOpts{
		Page:    page,
		PerPage: opdsPageSize,
		Sort:    "added",
	}
	books, _, err := s.DB.ListBooks(r.Context(), opts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feed := opds.NewAcquisitionFeed("urn:incipit:newest", "Newest Books", "/opds/newest")
	for _, book := range books {
		feed.AddEntry(opds.BookToEntry(book))
	}

	// Add next link if there are more pages.
	if len(books) == opdsPageSize {
		feed.AddNextLink("/opds/newest?page=" + strconv.Itoa(page+1))
	}

	writeOPDS(w, opds.ContentAcquisition, feed)
}

// handleOPDSByAuthorBooks handles GET /opds/byauthor/{author} — acquisition
// feed of books by a specific author.
func (s *Server) handleOPDSByAuthorBooks(w http.ResponseWriter, r *http.Request) {
	author, err := url.PathUnescape(chi.URLParam(r, "author"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	page := parsePage(r)
	offset := (page - 1) * opdsPageSize

	books, total, err := s.DB.ListBooksByAuthor(r.Context(), author, opdsPageSize, offset)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feedID := "urn:incipit:byauthor:" + author
	selfHref := "/opds/byauthor/" + url.PathEscape(author)
	feed := opds.NewAcquisitionFeed(feedID, "Books by "+author, selfHref)
	for _, book := range books {
		feed.AddEntry(opds.BookToEntry(book))
	}

	if total > offset+opdsPageSize {
		feed.AddNextLink(selfHref + "?page=" + strconv.Itoa(page+1))
	}

	writeOPDS(w, opds.ContentAcquisition, feed)
}

// handleOPDSBySeriesBooks handles GET /opds/byseries/{series} — acquisition
// feed of books in a series, ordered by series_index.
func (s *Server) handleOPDSBySeriesBooks(w http.ResponseWriter, r *http.Request) {
	series, err := url.PathUnescape(chi.URLParam(r, "series"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	page := parsePage(r)
	offset := (page - 1) * opdsPageSize

	books, total, err := s.DB.ListBooksBySeries(r.Context(), series, opdsPageSize, offset)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feedID := "urn:incipit:byseries:" + series
	selfHref := "/opds/byseries/" + url.PathEscape(series)
	feed := opds.NewAcquisitionFeed(feedID, series, selfHref)
	for _, book := range books {
		feed.AddEntry(opds.BookToEntry(book))
	}

	if total > offset+opdsPageSize {
		feed.AddNextLink(selfHref + "?page=" + strconv.Itoa(page+1))
	}

	writeOPDS(w, opds.ContentAcquisition, feed)
}

// handleOPDSByTagBooks handles GET /opds/bytag/{tag} — acquisition feed of
// books with a specific tag.
func (s *Server) handleOPDSByTagBooks(w http.ResponseWriter, r *http.Request) {
	tag, err := url.PathUnescape(chi.URLParam(r, "tag"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	page := parsePage(r)
	offset := (page - 1) * opdsPageSize

	books, total, err := s.DB.ListBooksByTag(r.Context(), tag, opdsPageSize, offset)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feedID := "urn:incipit:bytag:" + tag
	selfHref := "/opds/bytag/" + url.PathEscape(tag)
	feed := opds.NewAcquisitionFeed(feedID, "Tag: "+tag, selfHref)
	for _, book := range books {
		feed.AddEntry(opds.BookToEntry(book))
	}

	if total > offset+opdsPageSize {
		feed.AddNextLink(selfHref + "?page=" + strconv.Itoa(page+1))
	}

	writeOPDS(w, opds.ContentAcquisition, feed)
}

// handleOPDSSearch handles GET /opds/search?q={query} — acquisition feed
// of search results.
func (s *Server) handleOPDSSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		// Empty search — return empty acquisition feed.
		feed := opds.NewAcquisitionFeed("urn:incipit:search", "Search Results", "/opds/search")
		writeOPDS(w, opds.ContentAcquisition, feed)
		return
	}

	page := parsePage(r)
	offset := (page - 1) * opdsPageSize

	books, total, err := s.Searcher.Search(r.Context(), q, search.Opts{
		Limit:  opdsPageSize,
		Offset: offset,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	feed := opds.NewAcquisitionFeed("urn:incipit:search", "Search: "+q, "/opds/search")
	for _, book := range books {
		feed.AddEntry(opds.BookToEntry(book))
	}

	if total > offset+opdsPageSize {
		feed.AddNextLink("/opds/search?q=" + url.QueryEscape(q) + "&page=" + strconv.Itoa(page+1))
	}

	writeOPDS(w, opds.ContentAcquisition, feed)
}

// parsePage extracts the page query param, defaulting to 1.
func parsePage(r *http.Request) int {
	return atoiDefault(r.URL.Query().Get("page"), 1)
}
```

The Server struct needs a `Searcher` field. Add to the struct in
`internal/server/server.go`:

```go
type Server struct {
	DB        *db.DB
	Storage   *storage.Storage
	Searcher  search.Searcher
	Config    config.Config
	Handler   http.Handler
}
```

And in `New()`, initialize the Searcher:

```go
	s := &Server{
		DB:       database,
		Storage:  store,
		Searcher: &search.LikeSearcher{DB: database},
		Config:   cfg,
	}
```

> **Note:** `search.LikeSearcher` needs a `DB *db.DB` field to query
> SQLite. Check Phase 1's actual constructor — adjust to match. The key
> point is the Searcher is wired in `New()` and available to all handlers.

Register routes in `server.go`. In the authenticated group:

```go
		r.Get("/opds/newest", s.handleOPDSNewest)
		r.Get("/opds/byauthor/{author}", s.handleOPDSByAuthorBooks)
		r.Get("/opds/byseries/{series}", s.handleOPDSBySeriesBooks)
		r.Get("/opds/bytag/{tag}", s.handleOPDSByTagBooks)
		r.Get("/opds/search", s.handleOPDSSearch)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run "TestOPDSNewest|TestOPDSByAuthorBooks|TestOPDSBySeriesBooks|TestOPDSByTagBooks|TestOPDSSearch" -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/opds.go internal/server/opds_test.go internal/server/server.go
git commit -m "feat(server): OPDS acquisition feeds — newest, byauthor, byseries, bytag, search"
```

Questions before moving on?

---

### Task 10: OPDS Download — /opds/book/{id}/download

**Files:**
- Modify: `internal/server/opds.go` (add `handleOPDSDownload`)
- Modify: `internal/server/opds_test.go` (add tests)

**Interfaces:**
- Consumes: `db.GetBook(ctx, id)`, `storage.BookFilePath(id)`, `http.ServeFile`
- Produces: `s.handleOPDSDownload` handler

> **Go note: `http.ServeFile`.** This stdlib function serves a file from
> the filesystem with proper Content-Type (based on extension), Last-Modified,
> and supports range requests (for partial downloads). You pass the
> `http.ResponseWriter`, `*http.Request`, and the file path. It handles
> 404 if the file doesn't exist.
>
> **Go note: Content-Disposition.** To suggest a filename for the download,
> set `Content-Disposition: attachment; filename="..."` before serving.
> `http.ServeFile` doesn't set this automatically — you add it manually.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/opds_test.go`:

```go
func TestOPDSDownload(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	// Create a minimal EPUB file in storage.
	id := seedBook(t, srv, models.Book{Title: "Test", Author: "Author"})
	epubPath := createTestEPUB(t, srv.Storage, id)

	body := authedGet(t, srv.URL+"/opds/book/"+strconv.FormatInt(id, 10)+"/download", "testuser", "testpass")

	// The body should be the EPUB file content (a ZIP).
	if len(body) < 4 {
		t.Fatalf("download body too small: %d bytes", len(body))
	}
	// ZIP files start with "PK".
	if string(body[:2]) != "PK" {
		t.Errorf("download body doesn't start with PK (ZIP magic): %x", body[:4])
	}
}

func TestOPDSDownload_NotFound_404(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL+"/opds/book/99999/download", nil)
	req.SetBasicAuth("testuser", "testpass")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// createTestEPUB creates a minimal valid EPUB (ZIP) file in storage and
// returns its path.
func createTestEPUB(t *testing.T, store *storage.Storage, bookID int64) string {
	t.Helper()
	// Create a minimal ZIP file that looks like an EPUB.
	path := store.BookFilePath(bookID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	mimetype, _ := w.Create("mimetype")
	mimetype.Write([]byte("application/epub+zip"))
	w.Close()

	return path
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestOPDSDownload -v`
Expected: FAIL — handler doesn't exist, route not registered.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/opds.go`:

```go
// handleOPDSDownload handles GET /opds/book/{id}/download — serves the
// EPUB file. Auth is required (enforced by middleware).
func (s *Server) handleOPDSDownload(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	book, err := s.DB.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	path := s.Storage.BookFilePath(id)

	// Set content type and disposition before serving.
	w.Header().Set("Content-Type", "application/epub+zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="%s.epub"`,
		sanitizeFilename(book.Title),
	))

	http.ServeFile(w, r, path)
}

// sanitizeFilename replaces characters that are problematic in filenames.
func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"\"", "_",
		"?", "_",
		"*", "_",
		"|", "_",
		"<", "_",
		">", "_",
	)
	return replacer.Replace(s)
}
```

Register route in `server.go`. In the authenticated group:

```go
		r.Get("/opds/book/{id}/download", s.handleOPDSDownload)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestOPDSDownload -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/opds.go internal/server/opds_test.go internal/server/server.go
git commit -m "feat(server): OPDS book download endpoint"
```

Questions before moving on?

---

### Task 11: Web UI — Templates, embed.FS, Template Rendering

**Files:**
- Create: `web/templates/base.html`
- Create: `web/templates/index.html`
- Create: `web/templates/book.html`
- Create: `web/templates/upload.html`
- Create: `web/templates/login.html`
- Create: `internal/server/embed.go`
- Create: `internal/server/render.go`
- Create: `internal/server/web.go`
- Create: `internal/server/web_test.go`

**Interfaces:**
- Consumes: `models.Book`, `db.ListBooks`, `db.GetBook`, `embed.FS`
- Produces: `s.handleIndex`, `s.handleBookPage`, `s.handleUploadForm`, `s.handleLoginPage` handlers, template rendering helper

> **Go note: `embed.FS`.** The `//go:embed` directive embeds files into the
> Go binary at compile time. You declare `//go:embed web/templates
> web/static` above an `embed.FS` variable. The embedded files are read-only
> and available without any external filesystem — critical for the
> `FROM scratch` Docker image which has no OS.
>
> ```go
> //go:embed web/templates web/static
> var webFS embed.FS
> ```
>
> The directive must be in a `.go` file at the module root (or wherever the
> embedded directories are relative to). The `embed` package is stdlib.
>
> **Go note: `html/template`.** Go's stdlib template engine. It
> auto-escapes HTML to prevent XSS — any data injected into templates is
> escaped by context (HTML body, attribute, JavaScript, URL). This is a
> security feature, not optional.
>
> **Go vs other languages:** In Python, you'd use Jinja2 or Django
> templates — external libraries. In C#, Razor templates. In Go, the
> template engine is built into the stdlib. No npm install, no pip
> install — just `import "html/template"`. The tradeoff: Go's templates
> are less feature-rich (no template inheritance in the classic sense,
> though you can compose with `{{template "name" .}}`).
>
> **Go note: template composition.** Go templates use `{{define "name"}}`
> and `{{template "name" .}}` for composition. The base template defines
> a block, child templates override it. This is Go's version of template
> inheritance — simpler than Jinja's `{% block %}` / `{% extends %}` but
> covers the same ground.

- [ ] **Step 1: Write the failing test**

Create `internal/server/web_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestIndexPage_Renders(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	seedBook(t, srv, models.Book{Title: "Test Book", Author: "Test Author"})

	body := authedGet(t, srv.URL+"/", "testuser", "testpass")
	htmlStr := string(body)

	if !strings.Contains(htmlStr, "Test Book") {
		t.Error("index page missing book title")
	}
	if !strings.Contains(htmlStr, "Test Author") {
		t.Error("index page missing book author")
	}
}

func TestBookPage_Renders(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{
		Title:       "Detail Book",
		Author:      "Detail Author",
		Description: "A test description.",
	})

	body := authedGet(t, srv.URL+"/book/"+strconv.FormatInt(id, 10), "testuser", "testpass")
	htmlStr := string(body)

	if !strings.Contains(htmlStr, "Detail Book") {
		t.Error("book page missing title")
	}
	if !strings.Contains(htmlStr, "Detail Author") {
		t.Error("book page missing author")
	}
}

func TestUploadForm_Renders(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	body := authedGet(t, srv.URL+"/upload", "testuser", "testpass")
	htmlStr := string(body)

	if !strings.Contains(htmlStr, `<form`) {
		t.Error("upload page missing form element")
	}
	if !strings.Contains(htmlStr, `enctype="multipart/form-data"`) {
		t.Error("upload form missing multipart encoding")
	}
	if !strings.Contains(htmlStr, `type="file"`) {
		t.Error("upload form missing file input")
	}
}

func TestLoginPage_Renders(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	// Login page is served on 401 — but since we always auth via basic
	// auth (KOReader style), the login page is informational. Test that
	// it renders when accessed.
	body := authedGet(t, srv.URL+"/login", "testuser", "testpass")
	htmlStr := string(body)

	if !strings.Contains(htmlStr, "login") {
		t.Error("login page missing 'login' text")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run "TestIndexPage|TestBookPage|TestUploadForm|TestLoginPage" -v`
Expected: FAIL — templates don't exist, handlers not registered.

- [ ] **Step 3: Write minimal implementation**

Create the templates.

`web/templates/base.html`:

```html
{{define "base"}}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>{{block "title" .}}Incipit{{end}}</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <nav>
        <a href="/">Incipit</a>
        <a href="/upload">Upload</a>
        <form action="/" method="get" class="search">
            <input type="text" name="q" placeholder="Search..." value="{{.SearchQuery}}">
            <button type="submit">Search</button>
        </form>
    </nav>
    <main>
        {{block "content" .}}{{end}}
    </main>
    <footer>
        <p>Incipit — Self-Hosted Ebook Server</p>
    </footer>
</body>
</html>
{{end}}
```

`web/templates/index.html`:

```html
{{define "index"}}
{{template "base" .}}
{{end}}

{{define "title"}}Incipit Library{{end}}

{{define "content"}}
<div class="book-grid">
    {{range .Books}}
    <div class="book-card">
        <a href="/book/{{.ID}}">
            {{if .CoverPath}}
            <img src="/covers/{{.ID}}.jpg" alt="{{.Title}}">
            {{else}}
            <div class="no-cover">No Cover</div>
            {{end}}
            <h3>{{.Title}}</h3>
            <p>{{.Author}}</p>
            {{if .Series}}
            <p class="series">{{.Series}} #{{.SeriesIndex}}</p>
            {{end}}
        </a>
    </div>
    {{else}}
    <p>No books yet. Upload some!</p>
    {{end}}
</div>

{{if gt .TotalPages 1}}
<div class="pagination">
    {{if gt .Page 1}}
    <a href="/?page={{.PrevPage}}">Previous</a>
    {{end}}
    <span>Page {{.Page}} of {{.TotalPages}}</span>
    {{if lt .Page .TotalPages}}
    <a href="/?page={{.NextPage}}">Next</a>
    {{end}}
</div>
{{end}}
{{end}}
```

`web/templates/book.html`:

```html
{{define "book"}}
{{template "base" .}}
{{end}}

{{define "title"}}{{.Book.Title}} — Incipit{{end}}

{{define "content"}}
<div class="book-detail">
    {{if .Book.CoverPath}}
    <img src="/covers/{{.Book.ID}}.jpg" alt="Cover" class="cover-large">
    {{end}}
    <h1>{{.Book.Title}}</h1>
    <p class="author">by {{.Book.Author}}</p>
    {{if .Book.Series}}
    <p class="series">{{.Book.Series}} #{{.Book.SeriesIndex}}</p>
    {{end}}
    {{if .Book.Description}}
    <div class="description">{{.Book.Description}}</div>
    {{end}}
    <dl class="metadata">
        {{if .Book.ISBN}}<dt>ISBN</dt><dd>{{.Book.ISBN}}</dd>{{end}}
        {{if .Book.Publisher}}<dt>Publisher</dt><dd>{{.Book.Publisher}}</dd>{{end}}
        {{if .Book.Published}}<dt>Published</dt><dd>{{.Book.Published}}</dd>{{end}}
        {{if .Book.Pages}}<dt>Pages</dt><dd>{{.Book.Pages}}</dd>{{end}}
        {{if .Book.Rating}}<dt>Rating</dt><dd>{{.Book.Rating}}/5</dd>{{end}}
    </dl>
    {{if .Tags}}
    <div class="tags">
        {{range .Tags}}
        <span class="tag">{{.Name}}</span>
        {{end}}
    </div>
    {{end}}
    <div class="actions">
        <a href="/opds/book/{{.Book.ID}}/download" class="button">Download</a>
    </div>
</div>
{{end}}
```

`web/templates/upload.html`:

```html
{{define "upload"}}
{{template "base" .}}
{{end}}

{{define "title"}}Upload — Incipit{{end}}

{{define "content"}}
<h1>Upload EPUB</h1>
<form action="/upload" method="post" enctype="multipart/form-data">
    <label>
        EPUB file:
        <input type="file" name="epub" accept=".epub" required>
    </label>
    <button type="submit">Upload</button>
</form>
{{end}}
```

`web/templates/login.html`:

```html
{{define "login"}}
{{template "base" .}}
{{end}}

{{define "title"}}Login — Incipit{{end}}

{{define "content"}}
<h1>Login Required</h1>
<p>Incipit uses HTTP Basic Authentication. Configure your browser or
KOReader to send your username and password.</p>
<p>If you're seeing this, your client didn't send credentials.</p>
{{end}}
```

`web/static/style.css`:

```css
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: system-ui, sans-serif; line-height: 1.5; color: #333; max-width: 1200px; margin: 0 auto; padding: 1rem; }
nav { display: flex; gap: 1rem; align-items: center; border-bottom: 1px solid #ddd; padding-bottom: 1rem; margin-bottom: 2rem; }
nav .search { margin-left: auto; }
.book-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 1.5rem; }
.book-card { text-align: center; }
.book-card img { max-width: 100%; height: auto; }
.book-card h3 { font-size: 0.9rem; margin-top: 0.5rem; }
.book-card .series { font-size: 0.8rem; color: #666; }
.no-cover { background: #eee; aspect-ratio: 2/3; display: flex; align-items: center; justify-content: center; }
.book-detail { display: grid; grid-template-columns: 200px 1fr; gap: 2rem; }
.cover-large { max-width: 200px; }
.description { margin: 1rem 0; }
.tags { display: flex; gap: 0.5rem; flex-wrap: wrap; margin: 1rem 0; }
.tag { background: #eee; padding: 0.25rem 0.5rem; border-radius: 3px; font-size: 0.85rem; }
.pagination { margin-top: 2rem; display: flex; gap: 1rem; justify-content: center; }
.button { display: inline-block; padding: 0.5rem 1rem; background: #0066cc; color: white; text-decoration: none; border-radius: 4px; }
form { display: flex; flex-direction: column; gap: 1rem; max-width: 400px; }
input, button { padding: 0.5rem; }
```

Create `internal/server/embed.go`:

```go
package server

import "embed"

// webFS embeds the web templates and static files into the binary.
// This allows the FROM scratch Docker image to serve the web UI without
// any external filesystem.
//
//go:embed web/templates web/static
var webFS embed.FS
```

> **Go note: `//go:embed` placement.** The directive must be immediately
> above the variable declaration (no blank line). The embedded paths are
> relative to the Go source file containing the directive. Since
> `embed.go` is at `internal/server/embed.go`, the paths `web/templates`
> and `web/static` would resolve relative to `internal/server/` — which is
> WRONG. The embed directive looks relative to the package directory.
>
> **Important fix:** Put the embed directive in a file at the module root
> (e.g., `main.go` or a dedicated `embed.go` at `~/Repos/incipit/embed.go`)
> and pass it to the server, OR adjust paths. The cleanest approach:
> create the embed at the module root since `web/` is at the root.
>
> Let's put it at the module root instead. Create `embed.go` at the repo
> root (not `internal/server/`):

Create `embed.go` at module root:

```go
package main

import "embed"

// webFS embeds the web templates and static files into the binary.
// Must be at module root so embed paths resolve to web/ directory.
//
//go:embed web/templates web/static
var webFS embed.FS
```

Then the server receives `webFS` as a parameter. Update the Server struct
and `New()`:

In `internal/server/server.go`:

```go
type Server struct {
	DB       *db.DB
	Storage  *storage.Storage
	Searcher search.Searcher
	WebFS    embed.FS
	Config   config.Config
	Handler  http.Handler
	templates *template.Template
}
```

In `New()`:

```go
func New(cfg config.Config) (*Server, error) {
	// ... existing DB/storage setup ...

	templates, err := parseTemplates(webFS)
	if err != nil {
		return nil, err
	}

	s := &Server{
		DB:        database,
		Storage:   store,
		Searcher:  &search.LikeSearcher{DB: database},
		WebFS:     webFS,
		Config:    cfg,
		templates: templates,
	}

	s.Handler = s.router()
	return s, nil
}
```

Create `internal/server/render.go`:

```go
package server

import (
	"embed"
	"html/template"
	"net/http"
)

// parseTemplates parses all HTML templates from the embedded filesystem.
// Templates are parsed once at startup and reused for all requests.
func parseTemplates(webFS embed.FS) (*template.Template, error) {
	return template.New("").ParseFS(webFS, "templates/*.html")
}

// renderTemplate executes a template with data and writes to w.
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		// If template execution fails after partial write, we can't
		// change the status code. Log the error.
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// indexData is the data struct for the index page.
type indexData struct {
	Books      []models.Book
	Page       int
	TotalPages int
	PrevPage   int
	NextPage   int
	SearchQuery string
}

// bookPageData is the data struct for the book detail page.
type bookPageData struct {
	Book *models.Book
	Tags []models.Tag
}
```

Create `internal/server/web.go`:

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jason/incipit/internal/models"
)

// handleIndex handles GET / — renders the book grid with pagination.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	opts := parseListOpts(r)
	books, total, err := s.DB.ListBooks(r.Context(), opts)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	totalPages := (total + opts.PerPage - 1) / opts.PerPage
	if totalPages < 1 {
		totalPages = 1
	}

	data := indexData{
		Books:       books,
		Page:        opts.Page,
		TotalPages:  totalPages,
		PrevPage:    opts.Page - 1,
		NextPage:    opts.Page + 1,
		SearchQuery: opts.Query,
	}

	s.renderTemplate(w, "index", data)
}

// handleBookPage handles GET /book/{id} — renders the book detail page.
func (s *Server) handleBookPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	book, err := s.DB.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	tags, _ := s.DB.GetTagsForBook(r.Context(), id)

	data := bookPageData{
		Book: book,
		Tags: tags,
	}

	s.renderTemplate(w, "book", data)
}

// handleUploadForm handles GET /upload — renders the upload form.
func (s *Server) handleUploadForm(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "upload", nil)
}

// handleLoginPage handles GET /login — renders an informational login page.
// Auth is via basic auth headers, not a form. This page explains that.
func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "login", nil)
}
```

Register routes in `server.go`. In the authenticated group, replace the
placeholder `r.Get("/", s.notImplemented)` and add the web routes:

```go
		// Web UI
		r.Get("/", s.handleIndex)
		r.Get("/book/{id}", s.handleBookPage)
		r.Get("/upload", s.handleUploadForm)
		r.Get("/login", s.handleLoginPage)

		// Static files (embedded)
		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(s.WebFS.SubStatic()))))

		// JSON API
		r.Get("/api/books", s.handleListBooks)
		// ... existing routes ...
```

> **Note on `s.WebFS.SubStatic()`:** This needs a helper. `embed.FS` has a
> `Sub(dir)` method that returns a sub-filesystem. Create a helper:
>
> ```go
> func (webFS embed.FS) SubStatic() fs.FS {
>     sub, _ := fs.Sub(webFS, "static")
>     return sub
> }
> ```
>
> Actually, `http.FS` takes an `fs.FS`, and `embed.FS` implements `fs.FS`
> rooted at the embed path. To serve only `static/`, use
> `http.FS(fs.Sub(webFS, "static"))`. Let's inline this in the route:
>
> ```go
> staticFS, _ := fs.Sub(s.WebFS, "static")
> r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
> ```

Update the `embed.go` at root to export `webFS` and update `New()` to
accept it (or make it a package-level var that server imports). Since
`embed.go` is in `package main` and `server` is `package server`, the
server can't import from `main`. Two options:

1. Put `embed.go` in a separate package (e.g., `internal/web/web.go`)
2. Pass `webFS` as a parameter to `New()`

Option 2 is cleaner. Update `New()` signature:

```go
func New(cfg config.Config, webFS embed.FS) (*Server, error) {
```

And `main.go` passes `webFS` to it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run "TestIndexPage|TestBookPage|TestUploadForm|TestLoginPage" -v`
Expected: PASS (after fixing the embed path and passing webFS to New())

Run: `go vet ./internal/server/`
Expected: clean

Run: `gofmt -l internal/server/`
Expected: empty

- [ ] **Step 5: Commit**

```bash
git add web/templates/ web/static/style.css embed.go internal/server/render.go internal/server/web.go internal/server/web_test.go internal/server/server.go
git commit -m "feat(server): web UI templates with embed.FS, template rendering"
```

Questions before moving on?

---

### Task 12: Web UI — Static files, cover serving, EPUB file serving

**Files:**
- Modify: `internal/server/web.go` (add `handleCover`, `handleFile`)
- Modify: `internal/server/web_test.go` (add tests)

**Interfaces:**
- Consumes: `storage.CoverPath(id)`, `storage.BookFilePath(id)`, `db.GetBook(ctx, id)`
- Produces: `s.handleCover`, `s.handleFile` handlers

> **Go note: `http.ServeFile` with cache headers.** For cover images, set
> `Cache-Control: max-age=31536000` (1 year) before calling
> `http.ServeFile`. Covers don't change (a book's cover is immutable), so
> aggressive caching reduces server load. `http.ServeFile` adds
> `Last-Modified` and handles `If-Modified-Since` for 304 responses.
>
> **Go note: `http.ServeContent`.** For more control than `ServeFile`,
> use `http.ServeContent(w, r, name, modTime, content)` — you provide an
> `io.ReadSeeker` and it handles range requests, content type detection,
> and caching headers.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/web_test.go`:

```go
func TestCoverServing(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{Title: "T", Author: "A"})

	// Create a fake cover file.
	coverPath := srv.Storage.CoverPath(id)
	os.MkdirAll(filepath.Dir(coverPath), 0755)
	os.WriteFile(coverPath, []byte("FAKEJPEGDATA"), 0644)

	resp := authedGetRaw(t, srv.URL+"/covers/"+strconv.FormatInt(id, 10)+".jpg", "testuser", "testpass")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") == "" {
		t.Error("missing Cache-Control header")
	}
}

func TestCoverServing_NotFound_404(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	resp := authedGetRaw(t, srv.URL+"/covers/99999.jpg", "testuser", "testpass")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestFileServing(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	id := seedBook(t, srv, models.Book{Title: "T", Author: "A"})
	createTestEPUB(t, srv.Storage, id)

	resp := authedGetRaw(t, srv.URL+"/files/"+strconv.FormatInt(id, 10)+".epub", "testuser", "testpass")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body[:2]) != "PK" {
		t.Error("EPUB file doesn't start with PK (ZIP magic)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run "TestCoverServing|TestFileServing" -v`
Expected: FAIL — handlers don't exist, routes not registered.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/web.go`:

```go
// handleCover handles GET /covers/{id}.jpg — serves a book's cover image
// with aggressive cache headers (covers are immutable).
func (s *Server) handleCover(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	path := s.Storage.CoverPath(id)

	// Check file exists.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Serve a placeholder cover in the future. For now, 404.
		http.NotFound(w, r)
		return
	}

	// Covers are immutable — cache for 1 year.
	w.Header().Set("Cache-Control", "max-age=31536000")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, path)
}

// handleFile handles GET /files/{id}.epub — serves the EPUB file.
// Auth is required (enforced by middleware).
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Verify book exists in DB.
	_, err = s.DB.GetBook(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	path := s.Storage.BookFilePath(id)

	w.Header().Set("Content-Type", "application/epub+zip")
	http.ServeFile(w, r, path)
}
```

Register routes in `server.go`. In the authenticated group:

```go
		r.Get("/covers/{id}.jpg", s.handleCover)
		r.Get("/files/{id}.epub", s.handleFile)
```

> **Note on route patterns:** chi's `{id}` matches any path segment. To
> restrict to digits, use `{id:[0-9]+}`. The `.jpg` and `.epub` suffixes
> are literal in the pattern — chi matches them as part of the path.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run "TestCoverServing|TestFileServing" -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/web.go internal/server/web_test.go internal/server/server.go
git commit -m "feat(server): cover image and EPUB file serving with cache headers"
```

Questions before moving on?

---

### Task 13: Web UI — Upload Handler

**Files:**
- Modify: `internal/server/web.go` (add `handleUpload`)
- Modify: `internal/server/web_test.go` (add tests)

**Interfaces:**
- Consumes: `r.ParseMultipartForm(maxBytes)`, `r.FormFile("epub")`, `epub.Parse(path)`, `lookup.Lookup(ctx, isbn, title, author)`, `db.InsertBook(ctx, *Book)`, `storage.SaveBookFile(id, sourcePath)`, `storage.SaveCover(id, data)`, `storage.HashFile(path)`
- Produces: `s.handleUpload` handler

> **Go note: `r.ParseMultipartForm`.** This parses multipart form data
> (file uploads) from the request body. It takes a max memory limit —
> files larger than this are written to temp files on disk. `r.FormFile`
> returns the file header, the opened file, and an error. Always close
> the file with `defer file.Close()`.
>
> **Go note: `os.CreateTemp`.** For upload processing, save the uploaded
> file to a temp location first, parse it, then copy to storage. This
> separates parsing from storage. Use `os.CreateTemp("", "incipit-*.epub")`
> and `defer os.Remove(tempPath)` to clean up.

- [ ] **Step 1: Write the failing test**

Add to `internal/server/web_test.go`:

```go
func TestUpload(t *testing.T) {
	srv := newTestServerWithData(t)
	defer srv.Close()

	// Create a minimal EPUB in memory.
	epubData := createMinimalEPUB(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("epub", "test.epub")
	part.Write(epubData)
	writer.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/upload", body)
	req.SetBasicAuth("testuser", "testpass")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303 (redirect to index)", resp.StatusCode)
	}

	// Verify the book was added.
	books, total, _ := srv.DB.ListBooks(context.Background(), db.ListOpts{Page: 1, PerPage: 20})
	if total != 1 {
		t.Errorf("total books = %d, want 1", total)
	}
	if len(books) > 0 && books[0].Title == "" {
		t.Error("book title empty after upload")
	}
}

// createMinimalEPUB creates a minimal valid EPUB (ZIP) in memory.
func createMinimalEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// mimetype must be first and uncompressed.
	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mimetype, _ := w.CreateHeader(mh)
	mimetype.Write([]byte("application/epub+zip"))

	// container.xml
	container, _ := w.Create("META-INF/container.xml")
	container.Write([]byte(`<?xml version="1.0"?>
<container version="1.0">
  <rootfiles>
    <rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`))

	// content.opf with metadata
	opf, _ := w.Create("content.opf")
	opf.Write([]byte(`<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata>
    <dc:title>Upload Test Book</dc:title>
    <dc:creator>Test Author</dc:creator>
    <dc:identifier>urn:isbn:1234567890</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
</package>`))

	w.Close()
	return buf.Bytes()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestUpload -v`
Expected: FAIL — `handleUpload` doesn't exist, POST /upload returns 404.

- [ ] **Step 3: Write minimal implementation**

Add to `internal/server/web.go`:

```go
// maxUploadSize is the maximum EPUB upload size (50 MB).
const maxUploadSize = 50 << 20

// handleUpload handles POST /upload — accepts an EPUB file, parses
// metadata, looks up enriched metadata, saves to DB and storage.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "upload too large or invalid", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("epub")
	if err != nil {
		http.Error(w, "missing epub file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save uploaded file to temp location for processing.
	tempPath := filepath.Join(os.TempDir(), "incipit-upload-"+header.Filename)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tempPath)

	if _, err := io.Copy(tempFile, file); err != nil {
		tempFile.Close()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tempFile.Close()

	// Parse EPUB metadata.
	meta, err := epub.Parse(tempPath)
	if err != nil {
		http.Error(w, "invalid EPUB: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract ISBN (strip urn:isbn: prefix).
	isbn := stripISBNPrefix(meta.Identifier)

	// Lookup enriched metadata (best effort, non-fatal if fails).
	lookupResult, _ := lookup.Lookup(r.Context(), isbn, meta.Title, meta.Creator)

	// Build book record.
	book := &models.Book{
		Title:    meta.Title,
		Author:   meta.Creator,
		ISBN:     isbn,
		Language: meta.Language,
		Publisher: meta.Publisher,
	}
	if lookupResult != nil {
		if lookupResult.Title != "" {
			book.Title = lookupResult.Title
		}
		if lookupResult.Author != "" {
			book.Author = lookupResult.Author
		}
		book.Series = lookupResult.Series
		book.Description = lookupResult.Description
		book.Publisher = lookupResult.Publisher
		book.Published = lookupResult.Published
		book.Pages = lookupResult.Pages
		book.Rating = lookupResult.Rating
	}

	// Compute file hash.
	hash, err := s.Storage.HashFile(tempPath)
	if err != nil {
		book.FileHash = ""
	} else {
		book.FileHash = hash
	}

	// Get file size.
	fi, _ := os.Stat(tempPath)
	if fi != nil {
		book.FileSize = fi.Size()
	}

	// Insert into DB.
	id, err := s.DB.InsertBook(r.Context(), book)
	if err != nil {
		http.Error(w, "failed to save book: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Copy file to storage.
	if err := s.Storage.SaveBookFile(id, tempPath); err != nil {
		// Book record exists but file failed — non-fatal.
		// In production, log this.
	}

	// Download cover if available.
	if lookupResult != nil && lookupResult.CoverURL != "" {
		if coverData, err := fetchCover(lookupResult.CoverURL); err == nil {
			_ = s.Storage.SaveCover(id, coverData)
		}
	}

	// Redirect to the book detail page.
	http.Redirect(w, r, "/book/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// stripISBNPrefix removes urn:isbn: prefix and normalizes to digits.
func stripISBNPrefix(s string) string {
	s = strings.TrimPrefix(s, "urn:isbn:")
	s = strings.TrimPrefix(s, "urn:ISBN:")
	// Remove hyphens.
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// fetchCover downloads cover image bytes from a URL.
func fetchCover(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cover download: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

Register the route in `server.go`. In the authenticated group:

```go
		r.Post("/upload", s.handleUpload)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestUpload -v`
Expected: PASS

Run: `go vet ./internal/server/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/server/web.go internal/server/web_test.go internal/server/server.go
git commit -m "feat(server): EPUB upload handler with metadata parsing and lookup"
```

Questions before moving on?

---

### Task 14: `incipit serve` subcommand in main.go

**Files:**
- Modify: `main.go` (add `serve` subcommand)

**Interfaces:**
- Consumes: `config.Load()`, `server.New(cfg, webFS)`, `server.Run()`
- Produces: Working `incipit serve` command

> **Go note: subcommand dispatch.** Go doesn't have a built-in subcommand
> framework. The standard pattern: check `os.Args[1]`, switch on it, pass
> `os.Args[2:]` to the subcommand handler. Each subcommand builds its own
> `flag.FlagSet` for flag parsing. This is simple and explicit — no
> framework needed.
>
> **Go note: `log.Fatal`.** This is `log.Println` followed by `os.Exit(1)`.
> Use it for unrecoverable errors at the top level. Don't use it in
> library code — it exits the process, which is rude to callers.

- [ ] **Step 1: Write the failing test**

Testing CLI subcommands is hard without spawning subprocesses. Instead,
we test that the server starts and serves. The integration is covered by
the server tests. For this task, manual verification suffices.

Create or add to `main_test.go` (optional smoke test):

```go
//go:build integration

package main

import (
	"net/http"
	"os/exec"
	"testing"
	"time"
)

func TestServeCommand(t *testing.T) {
	// Start the server in a subprocess.
	cmd := exec.Command("go", "run", ".", "serve")
	cmd.Env = append(cmd.Environ, "INCIPIT_DB_PATH="+t.TempDir()+"/test.db", "INCIPIT_STORAGE_DIR="+t.TempDir(), "INCIPIT_PORT=18099")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	// Wait for server to start.
	time.Sleep(2 * time.Second)

	// Hit health endpoint.
	resp, err := http.Get("http://localhost:18099/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -tags integration ./ -run TestServeCommand -v`
Expected: FAIL — `serve` subcommand doesn't exist yet.

- [ ] **Step 3: Write minimal implementation**

Modify `main.go` to add the `serve` subcommand. Find the switch statement
and add:

```go
	case "serve":
		runServe()
```

Add the `runServe` function:

```go
// runServe starts the HTTP server.
func runServe() {
	cfg := config.Load()

	srv, err := server.New(cfg, webFS)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	if err := srv.Run(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

Add the necessary imports to `main.go`:

```go
import (
	// existing imports...
	"log"

	"github.com/jason/incipit/internal/server"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -tags integration ./ -run TestServeCommand -v`
Expected: PASS (server starts, /health returns 200)

Manual verification:
```bash
INCIPIT_DB_PATH=/tmp/test.db INCIPIT_STORAGE_DIR=/tmp INCIPT_PORT=18099 go run . serve
# In another terminal:
curl localhost:18099/health
# Expected: {"status":"ok"}
# Press Ctrl-C to stop, verify graceful shutdown message.
```

Run: `go vet ./...`
Expected: clean

Run: `gofmt -l .`
Expected: empty

- [ ] **Step 5: Commit**

```bash
git add main.go main_test.go
git commit -m "feat(cli): incipit serve subcommand starts HTTP server"
```

Questions before moving on?

---

### Task 15: internal/search — FTS5Searcher (optional upgrade)

**Files:**
- Create: `internal/search/fts5.go`
- Create: `internal/search/fts5_test.go`

**Interfaces:**
- Consumes: `search.Searcher` interface, `search.Opts{Limit, Offset}`, `models.Book`, `db.DB` (for FTS5 table creation + triggers)
- Produces: `search.FTS5Searcher` implementing `Searcher` with BM25 ranking

> **Go note: SQLite FTS5.** FTS5 (Full Text Search 5) is a SQLite extension
> that creates virtual tables for fast text search. `modernc.org/sqlite`
> includes FTS5 in the amalgamation. You create an FTS5 table with
> `CREATE VIRTUAL TABLE books_fts USING fts5(title, author, series,
> description, content='books', content_rowid='id')`. The `content=`
> option makes it an "external content" FTS5 table — it indexes the
> `books` table without duplicating data. Triggers keep the FTS index in
> sync when books are inserted/updated/deleted.
>
> **Go note: BM25 ranking.** FTS5 provides `bm25()` as a ranking function.
> In your query, use `ORDER BY bm25(books_fts)` for relevance ordering.
> Lower BM25 scores = better matches (it's a distance metric, not a
> similarity score). Use `ORDER BY bm25(books_fts) ASC` for best-first.
>
> **Go vs other languages:** This is a database feature, not a Go feature.
> The Go-specific part is that `modernc.org/sqlite` (pure Go) includes FTS5
> — you don't need a special build or CGO. In Python with `sqlite3` (which
> wraps the C library), FTS5 may or may not be compiled in depending on
> the OS. Go's pure-Go driver guarantees it's available.

- [ ] **Step 1: Write the failing test**

Create `internal/search/fts5_test.go`:

```go
package search

import (
	"context"
	"testing"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)

func TestFTS5Search_BasicQuery(t *testing.T) {
	database := newTestDB(t)
	searcher := &FTS5Searcher{DB: database}

	seedTestBooks(t, database,
		models.Book{Title: "Leviathan Wakes", Author: "Corey"},
		models.Book{Title: "Caliban's War", Author: "Corey"},
		models.Book{Title: "Foundation", Author: "Asimov"},
	)

	books, total, err := searcher.Search(context.Background(), "Leviathan",
		Opts{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(books) != 1 {
		t.Fatalf("books = %d, want 1", len(books))
	}
	if books[0].Title != "Leviathan Wakes" {
		t.Errorf("title = %q", books[0].Title)
	}
}

func TestFTS5Search_MultiFieldQuery(t *testing.T) {
	database := newTestDB(t)
	searcher := &FTS5Searcher{DB: database}

	seedTestBooks(t, database,
		models.Book{Title: "Leviathan Wakes", Author: "Corey"},
		models.Book{Title: "Foundation", Author: "Leviathan Author"},
	)

	// "Leviathan" should match both (one in title, one in author).
	books, total, _ := searcher.Search(context.Background(), "Leviathan",
		Opts{Limit: 10, Offset: 0})
	if total < 2 {
		t.Errorf("total = %d, want >= 2", total)
	}
	if len(books) < 2 {
		t.Errorf("books = %d, want >= 2", len(books))
	}
}

func TestFTS5Search_Ranking(t *testing.T) {
	database := newTestDB(t)
	searcher := &FTS5Searcher{DB: database}

	seedTestBooks(t, database,
		models.Book{Title: "The Expanse Leviathan", Author: "Corey"},
		models.Book{Title: "Leviathan Wakes", Author: "Corey"},
	)

	// "Leviathan Wakes" should rank higher than "The Expanse Leviathan"
	// because it's a closer match (title starts with the term).
	books, _, _ := searcher.Search(context.Background(), "Leviathan Wakes",
		Opts{Limit: 10, Offset: 0})
	if len(books) < 2 {
		t.Fatal("not enough results")
	}
	if books[0].Title != "Leviathan Wakes" {
		t.Errorf("top result = %q, want 'Leviathan Wakes'", books[0].Title)
	}
}

func TestFTS5Search_Pagination(t *testing.T) {
	database := newTestDB(t)
	searcher := &FTS5Searcher{DB: database}

	// Seed 10 books all matching "test".
	books := make([]models.Book, 10)
	for i := range books {
		books[i] = models.Book{Title: "Test Book " + strconv.Itoa(i), Author: "Author"}
	}
	seedTestBooks(t, database, books...)

	// Page 1, 3 per page.
	results, total, _ := searcher.Search(context.Background(), "test",
		Opts{Limit: 3, Offset: 0})
	if total < 10 {
		t.Errorf("total = %d, want >= 10", total)
	}
	if len(results) != 3 {
		t.Errorf("page 1 results = %d, want 3", len(results))
	}

	// Page 2.
	results, _, _ = searcher.Search(context.Background(), "test",
		Opts{Limit: 3, Offset: 3})
	if len(results) != 3 {
		t.Errorf("page 2 results = %d, want 3", len(results))
	}
}

// newTestDB creates a temp SQLite DB with migrations + FTS5 setup.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := database.SetupFTS5(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return database
}

// seedTestBooks inserts books into the DB.
func seedTestBooks(t *testing.T, database *db.DB, books ...models.Book) {
	t.Helper()
	for _, b := range books {
		_, err := database.InsertBook(context.Background(), &b)
		if err != nil {
			t.Fatalf("InsertBook: %v", err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -run TestFTS5 -v`
Expected: FAIL — `FTS5Searcher` doesn't exist, `db.SetupFTS5()` doesn't exist.

- [ ] **Step 3: Write minimal implementation**

Create `internal/search/fts5.go`:

```go
package search

import (
	"context"
	"fmt"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)

// FTS5Searcher implements the Searcher interface using SQLite FTS5
// full-text search with BM25 ranking. Drop-in replacement for LikeSearcher.
//
// Requires db.SetupFTS5() to have been called (creates the FTS5 virtual
// table and sync triggers). This is called during db.Migrate() or
// separately.
type FTS5Searcher struct {
	DB *db.DB
}

// Search executes a full-text search query against the FTS5 index and
// returns matching books with BM25 ranking (best matches first).
func (s *FTS5Searcher) Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error) {
	if q == "" {
		return []models.Book{}, 0, nil
	}

	// Build the FTS5 query. FTS5 uses MATCH operator.
	// Prefix matching: append * to each token for partial matches.
	ftsQuery := buildFTSQuery(q)

	limit := opts.Limit
	if limit == 0 {
		limit = 20
	}

	// Count total matches.
	var total int
	countSQL := `SELECT COUNT(*) FROM books_fts WHERE books_fts MATCH ?`
	if err := s.DB.QueryRowContext(ctx, countSQL, ftsQuery).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("fts count: %w", err)
	}

	// Fetch books with BM25 ranking.
	// bm25() returns a score where LOWER = BETTER (it's a distance).
	// So ORDER BY bm25(books_fts) ASC for best-first.
	query := `
		SELECT b.id, b.title, b.title_sort, b.author, b.author_sort,
		       b.series, b.series_index, b.isbn, b.description,
		       b.publisher, b.published, b.pages, b.rating,
		       b.cover_path, b.file_path, b.file_hash, b.file_size,
		       b.added, b.updated
		FROM books_fts f
		JOIN books b ON b.id = f.rowid
		WHERE books_fts MATCH ?
		ORDER BY bm25(books_fts) ASC
		LIMIT ? OFFSET ?`

	rows, err := s.DB.QueryContext(ctx, query, ftsQuery, limit, opts.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("fts query: %w", err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var b models.Book
		err := rows.Scan(
			&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort,
			&b.Series, &b.SeriesIndex, &b.ISBN, &b.Description,
			&b.Publisher, &b.Published, &b.Pages, &b.Rating,
			&b.CoverPath, &b.FilePath, &b.FileHash, &b.FileSize,
			&b.Added, &b.Updated,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		books = append(books, b)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if books == nil {
		books = []models.Book{}
	}

	return books, total, nil
}

// buildFTSQuery converts a user search string to an FTS5 MATCH expression.
// "Leviathan Wakes" → "Leviathan* Wakes*" (prefix matching on each token).
func buildFTSQuery(q string) string {
	tokens := strings.Fields(q)
	for i, token := range tokens {
		tokens[i] = token + "*"
	}
	return strings.Join(tokens, " ")
}
```

Add `SetupFTS5()` to `internal/db`. Create or modify
`internal/db/fts5.go`:

```go
package db

import "context"

// SetupFTS5 creates the FTS5 virtual table and sync triggers over the
// books table. Idempotent — safe to call multiple times.
//
// The FTS5 table is an "external content" table — it indexes the books
// table without duplicating data. Triggers keep the index in sync on
// INSERT, UPDATE, and DELETE.
func (db *DB) SetupFTS5() error {
	ctx := context.Background()

	// Create the FTS5 virtual table.
	_, err := db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS books_fts USING fts5(
			title, author, series, description,
			content='books', content_rowid='id'
		)
	`)
	if err != nil {
		return err
	}

	// Sync trigger: after INSERT into books, insert into FTS index.
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER IF NOT EXISTS books_fts_ai AFTER INSERT ON books BEGIN
			INSERT INTO books_fts(rowid, title, author, series, description)
			VALUES (new.id, new.title, new.author, new.series, new.description)
		END
	`)
	if err != nil {
		return err
	}

	// Sync trigger: after DELETE on books, delete from FTS index.
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER IF NOT EXISTS books_fts_ad AFTER DELETE ON books BEGIN
			INSERT INTO books_fts(books_fts, rowid, title, author, series, description)
			VALUES ('delete', old.id, old.title, old.author, old.series, old.description)
		END
	`)
	if err != nil {
		return err
	}

	// Sync trigger: after UPDATE on books, update FTS index.
	_, err = db.ExecContext(ctx, `
		CREATE TRIGGER IF NOT EXISTS books_fts_au AFTER UPDATE ON books BEGIN
			INSERT INTO books_fts(books_fts, rowid, title, author, series, description)
			VALUES ('delete', old.id, old.title, old.author, old.series, old.description)
			INSERT INTO books_fts(rowid, title, author, series, description)
			VALUES (new.id, new.title, new.author, new.series, new.description)
		END
	`)
	if err != nil {
		return err
	}

	// Backfill: index all existing books.
	_, err = db.ExecContext(ctx, `
		INSERT INTO books_fts(books_fts) VALUES ('rebuild')
	`)
	return err
}
```

> **Note on `db.QueryRowContext` / `db.QueryContext` / `db.ExecContext`:**
> These are methods on `*db.DB` that wrap `*sql.DB`'s equivalent methods.
> If Phase 1's `db.DB` doesn't expose these, add wrapper methods that
> delegate to the underlying `*sql.DB`. The `FTS5Searcher` needs query
> access — either expose `QueryContext`/`QueryRowContext` on `db.DB`, or
> give `FTS5Searcher` access to the raw `*sql.DB`. Exposing wrapper methods
> is cleaner and maintains the "all SQL goes through db package" rule.
>
> **Note:** `SetupFTS5()` should be called during `Migrate()` if the FTS5
> searcher is the default. Or called separately when switching from
> `LikeSearcher` to `FTS5Searcher`. For the optional upgrade path, call
> it in `Migrate()` and keep `LikeSearcher` as the default until the FTS5
> path is verified.

Call `SetupFTS5()` in `db.Migrate()` (add at the end of Migrate):

```go
func (db *DB) Migrate() error {
	// ... existing migration code ...

	// Setup FTS5 (idempotent — safe even if FTS5Searcher isn't used).
	if err := db.SetupFTS5(); err != nil {
		return err
	}

	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/search/ -run TestFTS5 -v`
Expected: PASS

Run: `go vet ./internal/search/ ./internal/db/`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/search/fts5.go internal/search/fts5_test.go internal/db/fts5.go internal/db/db.go
git commit -m "feat(search): FTS5Searcher with BM25 ranking, drop-in Searcher upgrade"
```

Questions before moving on?

---

## Phase 2 Completion Checklist

After all 15 tasks, verify the entire Phase 2 works end-to-end:

- [ ] `go vet ./...` — clean
- [ ] `gofmt -l .` — empty
- [ ] `go test ./...` — all passing

Manual verification:

- [ ] `INCIPIT_DB_PATH=/tmp/test.db INCIPIT_STORAGE_DIR=/tmp INCIPIT_PORT=8080 go run . init`
- [ ] `go run . add-user --username admin --password secret`
- [ ] `go run . add ~/some-book.epub`
- [ ] `go run . serve`
- [ ] Browser: `http://localhost:8080/` (basic auth with admin/secret) — see book grid
- [ ] Browser: click a book — see detail page
- [ ] Browser: `/upload` — upload a new EPUB — see it appear
- [ ] `curl -u admin:secret localhost:8080/api/books` — JSON list
- [ ] `curl -u admin:secret localhost:8080/opds` — OPDS root XML
- [ ] `curl -u admin:secret localhost:8080/opds/newest` — acquisition feed
- [ ] `curl -u admin:secret localhost:8080/opds/book/1/download > book.epub` — download works
- [ ] KOReader: add OPDS catalog `http://localhost:8080/opds` with credentials — browse and download a book
- [ ] Ctrl-C the server — verify "shutting down..." and "bye" messages

---

## Phase 2 → Phase 3 Handoff

Phase 2 produces the full web server. Phase 3 builds on it:

| Phase 3 needs from Phase 2 |
|---|
| `server.Server` with auth middleware (for sync endpoints) |
| OPDS feed generation (for KOReader integration) |
| `search.Searcher` interface (for potential search upgrades) |
| Web UI templates + upload (for tag/series management UI) |
| JSON API endpoints (for programmatic access) |
| `incipit serve` command (deployment foundation) |

Phase 3 adds: `internal/server/sync.go` (KOReader progress sync),
tag/series management UI, cover refinements, Dockerfile, deployment config.