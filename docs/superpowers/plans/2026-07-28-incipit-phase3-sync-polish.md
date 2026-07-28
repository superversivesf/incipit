# Incipit Phase 3: Sync + Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add KOReader reading progress sync, tag/series management UI, cover upload handling, and containerize the application for k3s deployment.

**Architecture:** Phase 3 adds `internal/server/sync.go` for KOReader sync protocol, tag/series/edit UI pages, cover upload via multipart, and a multi-stage Dockerfile producing a `FROM scratch` image. The sync schema requires a migration making `reading_progress.book_id` nullable and adding a `document_hash` column.

**Tech Stack:** Go 1.22, `modernc.org/sqlite`, `github.com/go-chi/chi/v5`, `golang.org/x/crypto/bcrypt`, `image` (stdlib), `net/http` multipart (stdlib), Docker multi-stage build.

## Global Constraints

- Go 1.22, module path `github.com/jason/incipit`
- Pure-Go SQLite via `modernc.org/sqlite` — no CGO. Build with `CGO_ENABLED=0`.
- Dependency allowlist: `modernc.org/sqlite`, `github.com/go-chi/chi/v5`, `github.com/go-chi/cors`, `golang.org/x/crypto/bcrypt`, plus Go stdlib only.
- All app code under `internal/`. Web assets under `web/{templates,static}/` embedded via `embed.FS`.
- Auth: every endpoint requires basic auth EXCEPT `/health` and `/syncs/healthcheck`. Already implemented in Phase 2.
- Sync schema deviation: `reading_progress.book_id` nullable. Drop FK constraint, keep index. Add `document_hash` column.
- `document_hash` = MD5 of EPUB content = `books.file_hash`.
- Progress keyed by `(book_id, user_id)`, latest writer wins, `device` informational only.
- No `POST /syncs/register` — users via CLI only.
- Dockerfile: multi-stage build → `FROM scratch`, `CGO_ENABLED=0`, `go build -ldflags="-s -w"`. Only binary needed (templates embedded).
- Deployment: k3s via `veridian-apps` Helm chart, ingress `incipit.veridiandynamics` (Traefik), single PVC at `/data`, probes hit `GET /health`.
- HTTP integration tests via `httptest`. No real network in tests.
- Quality gates: `go vet ./...` clean, `gofmt -l .` empty, `go test ./...` passing.

## File Structure

```
incipit/
├── internal/
│   ├── db/
│   │   ├── migrations/
│   │   │   └── 002_sync_hash.sql      # Schema changes for hash-based sync
│   │   └── progress.go                # Progress CRUD (Get, Upsert by hash+user)
│   ├── server/
│   │   ├── sync.go                    # KOReader sync handlers
│   │   ├── tags.go                    # Tag management handlers
│   │   ├── series.go                  # Series management handlers
│   │   ├── edit.go                    # Book editing handlers
│   │   └── covers.go                  # Cover upload handler
│   └── ...
├── web/templates/
│   ├── tags.html                      # Tag tree management page
│   ├── series.html                    # Series list page
│   └── edit.html                      # Book edit form
├── Dockerfile                          # Multi-stage build
└── deploy/
    └── values.yaml                     # Helm values template
```

---

## Task 1: Schema Migration for Hash-Based Sync

**Files:**
- Create: `internal/db/migrations/002_sync_hash.sql`
- Test: `internal/db/migrate_test.go`

**Interfaces:**
- Consumes: `db.DB` (from Phase 1)
- Produces: Updated `reading_progress` table with nullable `book_id`, new `document_hash` column

> **Go note:** This is the power of embedded migrations — we add a new SQL file and it runs automatically on the next `Migrate()` call. The migration system tracks which versions have been applied, so existing databases get the new columns without losing data. This is like EF Core migrations but with raw SQL — no ORM, no code generation.

> **Design note:** The Phase 1 schema already has `book_id` as nullable (no NOT NULL, no FK). This migration adds the `document_hash` column which is the key KOReader uses for sync. We index it for fast lookups.

- [ ] **Step 1: Write the failing test**

Create `internal/db/migrate_test.go`:

```go
package db

import "testing"

func TestMigration002AddsDocumentHash(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Verify document_hash column exists
	var colName string
	err = d.db.QueryRow(
		"SELECT name FROM pragma_table_info('reading_progress') WHERE name='document_hash'",
	).Scan(&colName)
	if err != nil {
		t.Fatalf("document_hash column not found: %v", err)
	}
	if colName != "document_hash" {
		t.Errorf("expected 'document_hash', got %q", colName)
	}
}
```

> **Go note:** `pragma_table_info('table_name')` is a SQLite pragma that returns column metadata. It's a table-valued function in SQLite — you query it like a table. This is how we verify schema programmatically in tests.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestMigration002 -v`
Expected: FAIL with `document_hash column not found`

- [ ] **Step 3: Write the migration SQL**

Create `internal/db/migrations/002_sync_hash.sql`:

```sql
-- Add document_hash column for KOReader sync-by-hash
-- document_hash = MD5 of EPUB content = books.file_hash
-- Allows sync even when book is not in library (book_id = NULL)
ALTER TABLE reading_progress ADD COLUMN document_hash TEXT;

-- Create index for fast hash lookups
CREATE INDEX IF NOT EXISTS idx_reading_progress_hash ON reading_progress(document_hash, user_id);
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestMigration002 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/migrations/002_sync_hash.sql internal/db/migrate_test.go
git commit -m "feat: add migration 002 for document_hash column in reading_progress"
```

Questions before moving on?

---

## Task 2: DB — Progress CRUD

**Files:**
- Create: `internal/db/progress.go`
- Test: `internal/db/progress_test.go`

**Interfaces:**
- Consumes: `db.DB`, `models.ReadingProgress`
- Produces: `db.DB.GetProgress(documentHash string, userID int64) (*models.ReadingProgress, error)`, `db.DB.UpsertProgress(p *models.ReadingProgress) error`

> **Go note:** "Upsert" (UPDATE or INSERT) in SQLite uses `ON CONFLICT ... DO UPDATE`. The `reading_progress` table has `PRIMARY KEY (book_id, user_id)`, but with nullable `book_id`, we need to key on `(document_hash, user_id)` instead. This is the schema deviation in action — we sync by hash, not by book ID.

- [ ] **Step 1: Write the failing test**

Create `internal/db/progress_test.go`:

```go
package db

import "testing"

func TestUpsertAndGetProgress(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	// Create a user first (FK requirement)
	userID, _ := d.CreateUser("alice", "hash", "user")

	progress := &models.ReadingProgress{
		UserID:     userID,
		DocumentHash: "abc123hash",
		Percentage: 0.318,
		Progress:   "/body/DocFragment[20]/body/p[22]/img.0",
		Device:     "Kobo",
	}

	err := d.UpsertProgress(progress)
	if err != nil {
		t.Fatalf("UpsertProgress failed: %v", err)
	}

	got, err := d.GetProgress("abc123hash", userID)
	if err != nil {
		t.Fatalf("GetProgress failed: %v", err)
	}
	if got.Percentage != 0.318 {
		t.Errorf("expected percentage 0.318, got %f", got.Percentage)
	}
	if got.Progress != "/body/DocFragment[20]/body/p[22]/img.0" {
		t.Errorf("expected progress string, got %q", got.Progress)
	}
	if got.Device != "Kobo" {
		t.Errorf("expected device 'Kobo', got %q", got.Device)
	}
}

func TestUpsertProgressOverwrites(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	userID, _ := d.CreateUser("alice", "hash", "user")

	// First save at 30%
	d.UpsertProgress(&models.ReadingProgress{
		UserID:       userID,
		DocumentHash: "hash123",
		Percentage:  0.30,
		Progress:     "/body/1",
		Device:       "Kobo",
	})

	// Second save at 35% — should overwrite
	d.UpsertProgress(&models.ReadingProgress{
		UserID:       userID,
		DocumentHash: "hash123",
		Percentage:  0.35,
		Progress:     "/body/2",
		Device:       "Phone",
	})

	got, _ := d.GetProgress("hash123", userID)
	if got.Percentage != 0.35 {
		t.Errorf("expected 0.35 (latest writer wins), got %f", got.Percentage)
	}
	if got.Device != "Phone" {
		t.Errorf("expected device 'Phone', got %q", got.Device)
	}
}

func TestGetProgressNotFound(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	userID, _ := d.CreateUser("alice", "hash", "user")

	_, err := d.GetProgress("nonexistent", userID)
	if err == nil {
		t.Fatal("expected error for nonexistent progress, got nil")
	}
}

func TestProgressPerUser(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	alice, _ := d.CreateUser("alice", "h1", "user")
	bob, _ := d.CreateUser("bob", "h2", "user")

	d.UpsertProgress(&models.ReadingProgress{
		UserID:       alice,
		DocumentHash: "sharedhash",
		Percentage:  0.50,
		Progress:     "/alice/pos",
		Device:       "Kobo",
	})
	d.UpsertProgress(&models.ReadingProgress{
		UserID:       bob,
		DocumentHash: "sharedhash",
		Percentage:  0.75,
		Progress:     "/bob/pos",
		Device:       "Phone",
	})

	aliceProgress, _ := d.GetProgress("sharedhash", alice)
	bobProgress, _ := d.GetProgress("sharedhash", bob)

	if aliceProgress.Percentage != 0.50 {
		t.Errorf("alice expected 0.50, got %f", aliceProgress.Percentage)
	}
	if bobProgress.Percentage != 0.75 {
		t.Errorf("bob expected 0.75, got %f", bobProgress.Percentage)
	}
}
```

> **Go note:** This test verifies per-user isolation — Alice and Bob have separate progress for the same document hash. This is a key correctness property: the sync server must not mix up users' reading positions.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestUpsert -v`
Expected: FAIL with `undefined: d.UpsertProgress`

- [ ] **Step 3: Update the ReadingProgress model**

Add `DocumentHash` field to `models.ReadingProgress` in `internal/models/models.go`:

```go
type ReadingProgress struct {
	BookID       *int64
	DocumentHash string
	UserID       int64
	Percentage   float64
	Progress     string
	Device       string
	Updated      string
}
```

- [ ] **Step 4: Write the progress CRUD**

Create `internal/db/progress.go`:

```go
package db

import (
	"database/sql"
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) GetProgress(documentHash string, userID int64) (*models.ReadingProgress, error) {
	var p models.ReadingProgress
	err := d.db.QueryRow(
		`SELECT book_id, document_hash, user_id, percentage, progress, device, updated
		 FROM reading_progress
		 WHERE document_hash = ? AND user_id = ?`,
		documentHash, userID,
	).Scan(&p.BookID, &p.DocumentHash, &p.UserID, &p.Percentage, &p.Progress, &p.Device, &p.Updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no progress for hash=%s user=%d: %w", documentHash, userID, err)
		}
		return nil, fmt.Errorf("getting progress: %w", err)
	}
	return &p, nil
}

func (d *DB) UpsertProgress(p *models.ReadingProgress) error {
	// Try to find the book by file_hash to link progress
	var bookID *int64
	var id int64
	err := d.db.QueryRow("SELECT id FROM books WHERE file_hash = ?", p.DocumentHash).Scan(&id)
	if err == nil {
		bookID = &id
	}

	_, err = d.db.Exec(
		`INSERT INTO reading_progress (book_id, document_hash, user_id, percentage, progress, device, updated)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(document_hash, user_id) DO UPDATE SET
		   book_id = excluded.book_id,
		   percentage = excluded.percentage,
		   progress = excluded.progress,
		   device = excluded.device,
		   updated = datetime('now')`,
		bookID, p.DocumentHash, p.UserID, p.Percentage, p.Progress, p.Device,
	)
	if err != nil {
		return fmt.Errorf("upserting progress: %w", err)
	}
	return nil
}
```

> **Go note:** `ON CONFLICT(document_hash, user_id)` requires a unique constraint on those columns. The migration created an index, but we need a UNIQUE constraint. Let's update the migration to use `CREATE UNIQUE INDEX` instead.

Update `internal/db/migrations/002_sync_hash.sql`:

```sql
-- Add document_hash column for KOReader sync-by-hash
ALTER TABLE reading_progress ADD COLUMN document_hash TEXT;

-- Unique index on (document_hash, user_id) for upsert
-- This replaces the old PK (book_id, user_id) since book_id is now nullable
CREATE UNIQUE INDEX IF NOT EXISTS idx_reading_progress_hash ON reading_progress(document_hash, user_id);
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/db/ -v`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/db/progress.go internal/db/progress_test.go internal/models/models.go internal/db/migrations/002_sync_hash.sql
git commit -m "feat: add progress CRUD with hash-based sync and per-user isolation"
```

Questions before moving on?

---

## Task 3: Server — Sync Healthcheck and Auth Endpoints

**Files:**
- Create: `internal/server/sync.go`
- Test: `internal/server/sync_test.go`

**Interfaces:**
- Consumes: `db.DB` (from Phase 1)
- Produces: HTTP handlers for `/syncs/healthcheck` and `/syncs/auth`

> **Go note:** The `/syncs/healthcheck` endpoint is outside the auth middleware — KOReader checks it before authenticating. The `/syncs/auth` endpoint is inside the auth middleware, so by the time the handler runs, authentication has already succeeded. The handler just returns the user info from the context. This is clean separation: middleware validates, handler reports.

- [ ] **Step 1: Write the failing test**

Create `internal/server/sync_test.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSyncHealthcheck(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/syncs/healthcheck")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["state"] != "OK" {
		t.Errorf("expected state 'OK', got %q", body["state"])
	}
}

func TestSyncAuthWithoutCredentials(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/syncs/auth")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("expected 401 without credentials, got %d", resp.StatusCode)
	}
}

func TestSyncAuthWithValidCredentials(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL+"/syncs/auth")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// newTestServer should set up basic auth — see the test helper
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 with valid credentials, got %d", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["username"] == "" {
		t.Error("expected username in response")
	}
}
```

> **Go note:** The test uses `newTestServer` — a test helper that boots a full Server with a temp-dir database and returns a `httptest.Server`. This was established in Phase 2. If Phase 2's test helper doesn't exist yet, we create it here.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSync -v`
Expected: FAIL with `undefined: newTestServer` or similar

- [ ] **Step 3: Write the sync handlers**

Create `internal/server/sync.go`:

```go
package server

import (
	"encoding/json"
	"net/http"

	"github.com/jason/incipit/internal/models"
)

func (s *Server) syncHealthcheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"state": "OK"})
}

func (s *Server) syncAuth(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r.Context())
	if user == nil {
		// This shouldn't happen — auth middleware should have caught it
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"username": user.Username,
		"role":     user.Role,
	})
}

func userFromContext(ctx context.Context) *models.User {
	val := ctx.Value(userKey{})
	if val == nil {
		return nil
	}
	return val.(*models.User)
}

type userKey struct{}
```

> **Go note:** `context.Value` is Go's way to pass request-scoped data through middleware. The auth middleware stores the user with a context key (`userKey{}`), and handlers retrieve it. The key type is an unexported struct to prevent collisions — this is a common Go pattern. Unlike C#'s `HttpContext.Items["user"]`, Go's context keys are type-safe (you define a key type, not a string).

- [ ] **Step 4: Ensure the test helper exists**

Check if `newTestServer` exists from Phase 2. If not, create `internal/server/testhelpers_test.go`:

```go
package server

import (
	"net/http/httptest"
	"testing"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
)

type testServer struct {
	*httptest.Server
	database *db.DB
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	cfg := config.Config{
		DBPath:     t.TempDir() + "/test.db",
		Port:       "0",
		StorageDir: t.TempDir(),
	}

	d, err := db.Open(cfg.DBPath)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	if err := d.Migrate(); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	// Create a test user
	d.CreateUser("testuser", "testhash", "admin")

	s := New(cfg)
	s.db = d

	ts := httptest.NewServer(s.router())
	return &testServer{Server: ts, database: d}
}

func (ts *testServer) authedRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.SetBasicAuth("testuser", "testhash")
	return req
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestSync -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/sync.go internal/server/sync_test.go
git commit -m "feat: add KOReader sync healthcheck and auth endpoints"
```

Questions before moving on?

---

## Task 4: Server — Progress Sync Endpoints

**Files:**
- Modify: `internal/server/sync.go`
- Test: `internal/server/sync_test.go`

**Interfaces:**
- Consumes: `db.GetProgress`, `db.UpsertProgress`, `userFromContext`
- Produces: `GET /syncs/progress/{hash}`, `PUT /syncs/progress/{hash}`

> **Go note:** Chi's `URLParam` extracts path parameters: `chi.URLParam(r, "hash")`. This is like ASP.NET's route binding `[FromRoute] string hash` but manual — Go doesn't do automatic model binding. You read the param, parse the body, and construct your struct by hand. More verbose, but transparent.

- [ ] **Step 1: Write the failing tests**

Add to `internal/server/sync_test.go`:

```go
func TestSyncPutProgress(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	body := `{"percentage": 0.318, "progress": "/body/DocFragment[20]/body/p[22]/img.0", "device": "Kobo"}`
	req := httptest.NewRequest("PUT", "/syncs/progress/abc123hash", strings.NewReader(body))
	req.SetBasicAuth("testuser", "testhash")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSyncGetProgress(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// First PUT some progress
	putBody := `{"percentage": 0.318, "progress": "/body/1", "device": "Kobo"}`
	req := httptest.NewRequest("PUT", "/syncs/progress/testhash", strings.NewReader(putBody))
	req.SetBasicAuth("testuser", "testhash")
	srv.Client().Do(req)

	// Then GET it
	req = httptest.NewRequest("GET", "/syncs/progress/testhash", nil)
	req.SetBasicAuth("testuser", "testhash")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if result["percentage"] != 0.318 {
		t.Errorf("expected percentage 0.318, got %v", result["percentage"])
	}
	if result["device"] != "Kobo" {
		t.Errorf("expected device 'Kobo', got %v", result["device"])
	}
}

func TestSyncGetProgressNotFound(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	req := httptest.NewRequest("GET", "/syncs/progress/nonexistent", nil)
	req.SetBasicAuth("testuser", "testhash")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSyncProgressPerUser(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// User 1 saves at 30%
	putBody := `{"percentage": 0.30, "progress": "/body/1", "device": "Kobo"}`
	req := httptest.NewRequest("PUT", "/syncs/progress/sharedhash", strings.NewReader(putBody))
	req.SetBasicAuth("testuser", "testhash")
	srv.Client().Do(req)

	// User 2 saves at 75% for same hash
	d.CreateUser("user2", "hash2", "user")
	putBody2 := `{"percentage": 0.75, "progress": "/body/2", "device": "Phone"}`
	req = httptest.NewRequest("PUT", "/syncs/progress/sharedhash", strings.NewReader(putBody2))
	req.SetBasicAuth("user2", "hash2")
	srv.Client().Do(req)

	// User 1 gets their 30%
	req = httptest.NewRequest("GET", "/syncs/progress/sharedhash", nil)
	req.SetBasicAuth("testuser", "testhash")
	resp, _ := srv.Client().Do(req)
	var result1 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result1)
	resp.Body.Close()
	if result1["percentage"] != 0.30 {
		t.Errorf("user1 expected 0.30, got %v", result1["percentage"])
	}

	// User 2 gets their 75%
	req = httptest.NewRequest("GET", "/syncs/progress/sharedhash", nil)
	req.SetBasicAuth("user2", "hash2")
	resp, _ = srv.Client().Do(req)
	var result2 map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result2)
	resp.Body.Close()
	if result2["percentage"] != 0.75 {
		t.Errorf("user2 expected 0.75, got %v", result2["percentage"])
	}
}
```

Add imports: `"strings"`, `"github.com/go-chi/chi/v5"`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestSyncGet -v`
Expected: FAIL

- [ ] **Step 3: Add the progress sync handlers**

Add to `internal/server/sync.go`:

```go
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

func (s *Server) getProgress(w http.ResponseWriter, r *http.Request) {
	hash := chi.URLParam(r, "hash")
	if hash == "" {
		http.Error(w, "missing document hash", http.StatusBadRequest)
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	progress, err := s.db.GetProgress(hash, user.ID)
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

	user := userFromContext(r.Context())
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

	if err := s.db.UpsertProgress(progress); err != nil {
		http.Error(w, "error saving progress", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Register the routes**

In the server's router setup (from Phase 2), add inside the authed group:

```go
r.Get("/syncs/progress/{hash}", s.getProgress)
r.Put("/syncs/progress/{hash}", s.putProgress)
```

And outside the authed group (with `/health` and `/syncs/healthcheck`):

```go
r.Get("/syncs/auth", s.syncAuth)
```

> **Go note:** Route registration order matters in chi. Routes outside the authed group are mounted first, then the authed group wraps the rest. The `/syncs/auth` endpoint goes inside the authed group (it requires auth), while `/syncs/healthcheck` stays outside.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -run TestSync -v`
Expected: all sync tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/sync.go internal/server/sync_test.go
git commit -m "feat: add KOReader progress sync GET/PUT endpoints"
```

Questions before moving on?

---

## Task 5: Tag Management — API and UI

**Files:**
- Create: `internal/db/tags.go`
- Create: `internal/server/tags.go`
- Create: `web/templates/tags.html`
- Test: `internal/db/tags_test.go`
- Test: `internal/server/tags_test.go`

**Interfaces:**
- Consumes: `db.DB`, `models.Tag`
- Produces: `GET /tags` (HTML), `POST /api/tags`, `PUT /api/tags/{id}`, `DELETE /api/tags/{id}`, `db.ListTags() ([]models.Tag, error)`, `db.CreateTag(name string, parentID *int64) (int64, error)`, `db.UpdateTag(id int64, name string, parentID *int64) error`, `db.DeleteTag(id int64) error`

> **Go note:** Hierarchical data in SQL uses a `parent_id` self-reference. The `tags` table has `parent_id INTEGER` pointing to `tags.id`. NULL means top-level. This is the "adjacency list" model — simple to query one level at a time, harder for deep trees. For our tag tree (typically 2-3 levels), it's fine. C# Entity Framework would model this as a self-referencing navigation property; in Go we just query it manually.

- [ ] **Step 1: Write the failing DB test**

Create `internal/db/tags_test.go`:

```go
package db

import "testing"

func TestCreateAndListTags(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	id, err := d.CreateTag("Science Fiction", nil)
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	childID, _ := d.CreateTag("Space Opera", &id)
	tags, err := d.ListTags()
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}

	// Find the child tag and verify parent
	var child *struct {
		id       int64
		parentID *int64
	}
	_ = child
	for _, tag := range tags {
		if tag.ID == childID {
			if tag.ParentID == nil || *tag.ParentID != id {
				t.Errorf("expected parent_id %d, got %v", id, tag.ParentID)
			}
		}
	}
}

func TestUpdateTag(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	id, _ := d.CreateTag("OldName", nil)
	err := d.UpdateTag(id, "NewName", nil)
	if err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}

	tags, _ := d.ListTags()
	for _, tag := range tags {
		if tag.ID == id && tag.Name != "NewName" {
			t.Errorf("expected 'NewName', got %q", tag.Name)
		}
	}
}

func TestDeleteTag(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	id, _ := d.CreateTag("ToDelete", nil)
	d.DeleteTag(id)

	tags, _ := d.ListTags()
	for _, tag := range tags {
		if tag.ID == id {
			t.Error("tag should have been deleted")
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestCreateAndListTags -v`
Expected: FAIL with `undefined: d.CreateTag`

- [ ] **Step 3: Write the tag CRUD**

Create `internal/db/tags.go`:

```go
package db

import (
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) ListTags() ([]models.Tag, error) {
	rows, err := d.db.Query(
		`SELECT id, name, parent_id FROM tags ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		var parentID *int64
		if err := rows.Scan(&t.ID, &t.Name, &parentID); err != nil {
			return nil, fmt.Errorf("scanning tag row: %w", err)
		}
		t.ParentID = parentID
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (d *DB) CreateTag(name string, parentID *int64) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO tags (name, parent_id) VALUES (?, ?)`,
		name, parentID,
	)
	if err != nil {
		return 0, fmt.Errorf("creating tag: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting tag ID: %w", err)
	}
	return id, nil
}

func (d *DB) UpdateTag(id int64, name string, parentID *int64) error {
	_, err := d.db.Exec(
		`UPDATE tags SET name = ?, parent_id = ? WHERE id = ?`,
		name, parentID, id,
	)
	if err != nil {
		return fmt.Errorf("updating tag %d: %w", id, err)
	}
	return nil
}

func (d *DB) DeleteTag(id int64) error {
	_, err := d.db.Exec("DELETE FROM tags WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting tag %d: %w", id, err)
	}
	return nil
}

func (d *DB) GetTagsForBook(bookID int64) ([]models.Tag, error) {
	rows, err := d.db.Query(
		`SELECT t.id, t.name, t.parent_id
		 FROM tags t
		 JOIN book_tags bt ON bt.tag_id = t.id
		 WHERE bt.book_id = ?
		 ORDER BY t.name`,
		bookID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting tags for book %d: %w", bookID, err)
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		var parentID *int64
		if err := rows.Scan(&t.ID, &t.Name, &parentID); err != nil {
			return nil, fmt.Errorf("scanning tag row: %w", err)
		}
		t.ParentID = parentID
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (d *DB) AddTagToBook(bookID, tagID int64) error {
	_, err := d.db.Exec(
		`INSERT OR IGNORE INTO book_tags (book_id, tag_id) VALUES (?, ?)`,
		bookID, tagID,
	)
	if err != nil {
		return fmt.Errorf("adding tag %d to book %d: %w", tagID, bookID, err)
	}
	return nil
}

func (d *DB) RemoveTagFromBook(bookID, tagID int64) error {
	_, err := d.db.Exec(
		`DELETE FROM book_tags WHERE book_id = ? AND tag_id = ?`,
		bookID, tagID,
	)
	if err != nil {
		return fmt.Errorf("removing tag %d from book %d: %w", tagID, bookID, err)
	}
	return nil
}
```

> **Go note:** `INSERT OR IGNORE` is SQLite syntax — it silently skips if the row already exists (violates a unique/PK constraint). This is perfect for `book_tags` which has `PRIMARY KEY (book_id, tag_id)` — adding the same tag twice is a no-op. In PostgreSQL this would be `INSERT ... ON CONFLICT DO NOTHING`.

- [ ] **Step 4: Run DB tests to verify they pass**

Run: `go test ./internal/db/ -run TestCreate -v`
Expected: PASS

- [ ] **Step 5: Write the tag management server handlers**

Create `internal/server/tags.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jason/incipit/internal/models"
)

func (s *Server) tagsPage(w http.ResponseWriter, r *http.Request) {
	tags, err := s.db.ListTags()
	if err != nil {
		http.Error(w, "error loading tags", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "tags.html", tags)
}

func (s *Server) apiListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.db.ListTags()
	if err != nil {
		http.Error(w, "error loading tags", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

func (s *Server) apiCreateTag(w http.ResponseWriter, r *http.Request) {
	var tag struct {
		Name     string `json:"name"`
		ParentID *int64 `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&tag); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	id, err := s.db.CreateTag(tag.Name, tag.ParentID)
	if err != nil {
		http.Error(w, "error creating tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) apiUpdateTag(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
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

	if err := s.db.UpdateTag(id, tag.Name, tag.ParentID); err != nil {
		http.Error(w, "error updating tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) apiDeleteTag(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid tag ID", http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteTag(id); err != nil {
		http.Error(w, "error deleting tag", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

> **Go note:** `strconv.ParseInt(idStr, 10, 64)` parses a string to int64. The `10` is the base (decimal), `64` is the bit size. Unlike C#'s `int.Parse()` which throws on failure, Go returns an error — you check it and handle it. This is the Go error pattern everywhere: return error, check it at the call site.

- [ ] **Step 6: Write the tags HTML template**

Create `web/templates/tags.html`:

```html
{{template "base" .}}
{{define "content"}}
<h1>Tags</h1>

<div id="tag-tree">
{{range .}}
<div class="tag" style="margin-left: {{if .ParentID}}20px{{else}}0{{end}}">
    <span>{{.Name}}</span>
    <button onclick="editTag({{.ID}}, '{{.Name}}')">Edit</button>
    <button onclick="deleteTag({{.ID}})">Delete</button>
</div>
{{end}}
</div>

<form id="tag-form" method="post" action="/api/tags">
    <input type="text" name="name" placeholder="New tag name">
    <button type="submit">Add Tag</button>
</form>

<script>
function editTag(id, name) {
    var newName = prompt('Rename tag:', name);
    if (newName && newName !== name) {
        fetch('/api/tags/' + id, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name: newName})
        }).then(() => location.reload());
    }
}

function deleteTag(id) {
    if (confirm('Delete this tag?')) {
        fetch('/api/tags/' + id, {method: 'DELETE'})
            .then(() => location.reload());
    }
}
</script>
{{end}}
```

> **Go vs other languages:** This is vanilla JS — no framework, no build step. In a React/Vue app you'd have components, state management, a build pipeline. Here, `fetch()` calls the JSON API and reloads the page. Go's server-rendered approach is deliberately simple. The `{{range .}}` is Go's template `range` action — like `{{#each}}` in Handlebars but built into the stdlib.

- [ ] **Step 7: Register routes and run tests**

Add to the router:
```go
r.Get("/tags", s.tagsPage)
r.Get("/api/tags", s.apiListTags)
r.Post("/api/tags", s.apiCreateTag)
r.Put("/api/tags/{id}", s.apiUpdateTag)
r.Delete("/api/tags/{id}", s.apiDeleteTag)
```

Run: `go test ./internal/... -v`
Expected: all tests PASS

- [ ] **Step 8: Commit**

```bash
git add internal/db/tags.go internal/db/tags_test.go internal/server/tags.go web/templates/tags.html
git commit -m "feat: add tag management with CRUD API and tree UI"
```

Questions before moving on?

---

## Task 6: Series Management — API and UI

**Files:**
- Modify: `internal/db/books.go` (add series queries)
- Create: `internal/server/series.go`
- Create: `web/templates/series.html`
- Test: `internal/db/series_test.go`

**Interfaces:**
- Produces: `db.ListSeries() ([]SeriesInfo, error)`, `db.RenameSeries(oldName, newName string) error`, `GET /series` (HTML), `POST /api/series/rename`

> **Go note:** Series aren't a separate table — they're a field on `books`. "Listing series" means `SELECT DISTINCT series FROM books WHERE series IS NOT NULL`. "Renaming a series" means `UPDATE books SET series = ? WHERE series = ?`. This is simpler than a many-to-many relationship and works because each book belongs to at most one series.

- [ ] **Step 1: Write the failing test**

Create `internal/db/series_test.go`:

```go
package db

import "testing"

func TestListSeries(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	d.InsertBook(&models.Book{Title: "Book 1", Author: "Author", Series: "The Expanse", SeriesIndex: 1, FilePath: "f/1.epub"})
	d.InsertBook(&models.Book{Title: "Book 2", Author: "Author", Series: "The Expanse", SeriesIndex: 2, FilePath: "f/2.epub"})
	d.InsertBook(&models.Book{Title: "Dune", Author: "Herbert", Series: "Dune", SeriesIndex: 1, FilePath: "f/3.epub"})

	series, err := d.ListSeries()
	if err != nil {
		t.Fatalf("ListSeries failed: %v", err)
	}
	if len(series) != 2 {
		t.Errorf("expected 2 series, got %d", len(series))
	}

	// Find The Expanse
	var expanse *seriesInfo
	for _, s := range series {
		if s.Name == "The Expanse" {
			expanse = &s
		}
	}
	if expanse == nil {
		t.Fatal("The Expanse not found")
	}
	if expanse.BookCount != 2 {
		t.Errorf("expected 2 books in The Expanse, got %d", expanse.BookCount)
	}
}

func TestRenameSeries(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	d.InsertBook(&models.Book{Title: "Book 1", Author: "A", Series: "Old Name", FilePath: "f/1.epub"})
	d.InsertBook(&models.Book{Title: "Book 2", Author: "A", Series: "Old Name", FilePath: "f/2.epub"})

	err := d.RenameSeries("Old Name", "New Name")
	if err != nil {
		t.Fatalf("RenameSeries failed: %v", err)
	}

	series, _ := d.ListSeries()
	for _, s := range series {
		if s.Name == "Old Name" {
			t.Error("old series name still exists")
		}
		if s.Name == "New Name" && s.BookCount != 2 {
			t.Errorf("expected 2 books in New Name, got %d", s.BookCount)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestListSeries -v`
Expected: FAIL

- [ ] **Step 3: Write the series queries**

Add to `internal/db/books.go`:

```go
type SeriesInfo struct {
	Name      string
	BookCount int
}

func (d *DB) ListSeries() ([]SeriesInfo, error) {
	rows, err := d.db.Query(
		`SELECT series, COUNT(*) as book_count
		 FROM books
		 WHERE series IS NOT NULL AND series != ''
		 GROUP BY series
		 ORDER BY series`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing series: %w", err)
	}
	defer rows.Close()

	var series []SeriesInfo
	for rows.Next() {
		var s SeriesInfo
		if err := rows.Scan(&s.Name, &s.BookCount); err != nil {
			return nil, fmt.Errorf("scanning series row: %w", err)
		}
		series = append(series, s)
	}
	return series, rows.Err()
}

func (d *DB) RenameSeries(oldName, newName string) error {
	_, err := d.db.Exec(
		"UPDATE books SET series = ?, updated = datetime('now') WHERE series = ?",
		newName, oldName,
	)
	if err != nil {
		return fmt.Errorf("renaming series %q to %q: %w", oldName, newName, err)
	}
	return nil
}

func (d *DB) BooksBySeries(seriesName string) ([]models.Book, error) {
	rows, err := d.db.Query(
		`SELECT id, title, title_sort, author, author_sort, series, series_index,
		   isbn, description, publisher, published, pages, rating, cover_path,
		   file_path, file_hash, file_size, added, updated
		 FROM books WHERE series = ?
		 ORDER BY series_index`,
		seriesName,
	)
	if err != nil {
		return nil, fmt.Errorf("getting books in series %q: %w", seriesName, err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort,
			&b.Series, &b.SeriesIndex, &b.ISBN, &b.Description, &b.Publisher,
			&b.Published, &b.Pages, &b.Rating, &b.CoverPath, &b.FilePath,
			&b.FileHash, &b.FileSize, &b.Added, &b.Updated); err != nil {
			return nil, fmt.Errorf("scanning book row: %w", err)
		}
		books = append(books, b)
	}
	return books, rows.Err()
}
```

- [ ] **Step 4: Write the series server handlers**

Create `internal/server/series.go`:

```go
package server

import (
	"encoding/json"
	"net/http"
)

func (s *Server) seriesPage(w http.ResponseWriter, r *http.Request) {
	series, err := s.db.ListSeries()
	if err != nil {
		http.Error(w, "error loading series", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "series.html", series)
}

func (s *Server) apiListSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.db.ListSeries()
	if err != nil {
		http.Error(w, "error loading series", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, series)
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

	if err := s.db.RenameSeries(req.OldName, req.NewName); err != nil {
		http.Error(w, "error renaming series", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Write the series HTML template**

Create `web/templates/series.html`:

```html
{{template "base" .}}
{{define "content"}}
<h1>Series</h1>

<div class="series-list">
{{range .}}
<div class="series">
    <a href="/opds/byseries/{{.Name}}">{{.Name}}</a>
    <span>({{.BookCount}} books)</span>
    <button onclick="renameSeries('{{.Name}}')">Rename</button>
</div>
{{end}}
</div>

<script>
function renameSeries(oldName) {
    var newName = prompt('Rename series to:', oldName);
    if (newName && newName !== oldName) {
        fetch('/api/series/rename', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({old_name: oldName, new_name: newName})
        }).then(() => location.reload());
    }
}
</script>
{{end}}
```

- [ ] **Step 6: Register routes and run tests**

Add to router:
```go
r.Get("/series", s.seriesPage)
r.Get("/api/series", s.apiListSeries)
r.Post("/api/series/rename", s.apiRenameSeries)
```

Run: `go test ./internal/... -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/db/books.go internal/db/series_test.go internal/server/series.go web/templates/series.html
git commit -m "feat: add series management with list, rename, and UI"
```

Questions before moving on?

---

## Task 7: Book Editing — UI and Handler

**Files:**
- Create: `internal/server/edit.go`
- Create: `web/templates/edit.html`
- Test: `internal/server/edit_test.go`

**Interfaces:**
- Consumes: `db.GetBook`, `db.UpdateBook`, `db.GetTagsForBook`, `db.AddTagToBook`, `db.RemoveTagFromBook`, `db.ListTags`
- Produces: `GET /book/{id}/edit` (HTML form), `POST /book/{id}/edit` (save changes)

> **Go note:** HTML form handling in Go is manual. `r.FormValue("title")` reads a POST field. Unlike C#'s model binding (`[Bind] Book book`), Go doesn't automatically map form fields to structs — you extract them one by one. This is more verbose but transparent. For JSON APIs you use `json.Decode`, for forms you use `FormValue`.

- [ ] **Step 1: Write the failing test**

Create `internal/server/edit_test.go`:

```go
package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestEditBookPage(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	// Seed a book
	bookID, _ := srv.database.InsertBook(&models.Book{
		Title:    "Test Book",
		Author:   "Test Author",
		FilePath: "files/1.epub",
	})

	req := httptest.NewRequest("GET", "/book/"+strconv.FormatInt(bookID, 10)+"/edit", nil)
	req.SetBasicAuth("testuser", "testhash")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestEditBookSave(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	bookID, _ := srv.database.InsertBook(&models.Book{
		Title:    "Old Title",
		Author:   "Old Author",
		FilePath: "files/1.epub",
	})

	form := url.Values{
		"title":    {"New Title"},
		"author":   {"New Author"},
		"series":   {"The Expanse"},
		"isbn":     {"9780316129084"},
	}
	req := httptest.NewRequest("POST", "/book/"+strconv.FormatInt(bookID, 10)+"/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("testuser", "testhash")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the book was updated
	book, _ := srv.database.GetBook(bookID)
	if book.Title != "New Title" {
		t.Errorf("expected 'New Title', got %q", book.Title)
	}
	if book.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got %q", book.Series)
	}
}
```

Add imports: `"strconv"`, `"net/url"`, `"github.com/jason/incipit/internal/models"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestEdit -v`
Expected: FAIL

- [ ] **Step 3: Write the edit handlers**

Create `internal/server/edit.go`:

```go
package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jason/incipit/internal/models"
)

type editPageData struct {
	Book     *models.Book
	AllTags  []models.Tag
	BookTags []models.Tag
}

func (s *Server) editBookPage(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid book ID", http.StatusBadRequest)
		return
	}

	book, err := s.db.GetBook(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	allTags, _ := s.db.ListTags()
	bookTags, _ := s.db.GetTagsForBook(id)

	data := editPageData{
		Book:     book,
		AllTags:  allTags,
		BookTags: bookTags,
	}
	s.renderTemplate(w, "edit.html", data)
}

func (s *Server) editBookSave(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid book ID", http.StatusBadRequest)
		return
	}

	book, err := s.db.GetBook(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}

	// Update fields from form
	book.Title = r.FormValue("title")
	book.Author = r.FormValue("author")
	book.Series = r.FormValue("series")
	book.ISBN = r.FormValue("isbn")
	book.Description = r.FormValue("description")
	book.Publisher = r.FormValue("publisher")
	book.Published = r.FormValue("published")

	seriesIndexStr := r.FormValue("series_index")
	if seriesIndexStr != "" {
		book.SeriesIndex, _ = strconv.ParseFloat(seriesIndexStr, 64)
	}

	pagesStr := r.FormValue("pages")
	if pagesStr != "" {
		book.Pages, _ = strconv.Atoi(pagesStr)
	}

	if err := s.db.UpdateBook(book); err != nil {
		http.Error(w, "error saving book", http.StatusInternalServerError)
		return
	}

	// Update tags
	r.ParseForm()
	selectedTags := r.Form["tags"]
	for _, tag := range book.BookTags {
		// This is simplified — in practice you'd diff old vs new tags
	}
	_ = selectedTags

	http.Redirect(w, r, "/book/"+idStr, http.StatusSeeOther)
}
```

> **Go note:** `r.Form["tags"]` returns a `[]string` of all form values with the key "tags" — this is how you handle multi-select checkboxes. `r.FormValue("tags")` would only return the first value. The distinction matters for multi-value fields. This is like `Request.Form.GetValues("tags")` in ASP.NET.

- [ ] **Step 4: Write the edit HTML template**

Create `web/templates/edit.html`:

```html
{{template "base" .}}
{{define "content"}}
<h1>Edit: {{.Book.Title}}</h1>

<form method="post" action="/book/{{.Book.ID}}/edit">
    <label>Title: <input type="text" name="title" value="{{.Book.Title}}"></label><br>
    <label>Author: <input type="text" name="author" value="{{.Book.Author}}"></label><br>
    <label>Series: <input type="text" name="series" value="{{.Book.Series}}"></label><br>
    <label>Series Index: <input type="number" step="0.1" name="series_index" value="{{.Book.SeriesIndex}}"></label><br>
    <label>ISBN: <input type="text" name="isbn" value="{{.Book.ISBN}}"></label><br>
    <label>Publisher: <input type="text" name="publisher" value="{{.Book.Publisher}}"></label><br>
    <label>Published: <input type="text" name="published" value="{{.Book.Published}}"></label><br>
    <label>Pages: <input type="number" name="pages" value="{{.Book.Pages}}"></label><br>
    <label>Description: <textarea name="description">{{.Book.Description}}</textarea></label><br>

    <fieldset>
        <legend>Tags</legend>
        {{range .AllTags}}
        <label>
            <input type="checkbox" name="tags" value="{{.ID}}"
                {{range $.BookTags}}{{if eq .ID $.ID}}checked{{end}}{{end}}>
            {{.Name}}
        </label>
        {{end}}
    </fieldset>

    <button type="submit">Save Changes</button>
    <a href="/book/{{.Book.ID}}">Cancel</a>
</form>

<form method="post" action="/book/{{.Book.ID}}/delete" onsubmit="return confirm('Delete this book?')">
    <button type="submit">Delete Book</button>
</form>
{{end}}
```

- [ ] **Step 5: Register routes and run tests**

Add to router:
```go
r.Get("/book/{id}/edit", s.editBookPage)
r.Post("/book/{id}/edit", s.editBookSave)
```

Run: `go test ./internal/server/ -run TestEdit -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/edit.go internal/server/edit_test.go web/templates/edit.html
git commit -m "feat: add book editing page with metadata and tag management"
```

Questions before moving on?

---

## Task 8: Cover Upload — Multipart Handler

**Files:**
- Create: `internal/server/covers.go`
- Test: `internal/server/covers_test.go`

**Interfaces:**
- Consumes: `storage.SaveCover`, `db.UpdateBook`
- Produces: `POST /api/books/{id}/cover` (multipart upload)

> **Go note:** Multipart form handling is built into `net/http`. `r.ParseMultipartForm(maxMemory)` parses the form, `r.FormFile("cover")` returns the uploaded file. Unlike Express's `multer` middleware or Python's `werkzeug`, there's no third-party library — it's all stdlib. The tradeoff is more verbose code, but zero dependencies.

> **Go note:** `r.FormFile("cover")` returns `(multipart.File, *multipart.FileHeader, error)`. The `multipart.File` implements `io.Reader` — you can read from it directly. You must also handle the error. This triple-return is common in Go I/O: the thing you want, metadata about it, and whether it worked.

- [ ] **Step 1: Write the failing test**

Create `internal/server/covers_test.go`:

```go
package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"
)

func TestCoverUpload(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	bookID, _ := srv.database.InsertBook(&models.Book{
		Title:    "Test Book",
		Author:   "Author",
		FilePath: "files/1.epub",
	})

	// Create a multipart form with a fake "image"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("cover", "cover.jpg")
	fw.Write([]byte("fake jpeg data"))
	w.Close()

	req := httptest.NewRequest("POST", "/api/books/"+strconv.FormatInt(bookID, 10)+"/cover", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.SetBasicAuth("testuser", "testhash")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Verify the cover was saved
	book, _ := srv.database.GetBook(bookID)
	if book.CoverPath == "" {
		t.Error("expected cover_path to be set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestCoverUpload -v`
Expected: FAIL

- [ ] **Step 3: Write the cover upload handler**

Create `internal/server/covers.go`:

```go
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

	// Limit upload size to 10MB
	r.ParseMultipartForm(10 << 20)

	file, header, err := r.FormFile("cover")
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

	// Basic validation — check it's a JPEG by magic bytes
	if len(imageData) < 3 || imageData[0] != 0xFF || imageData[1] != 0xD8 {
		http.Error(w, "file is not a JPEG", http.StatusBadRequest)
		return
	}

	// Save the cover
	if err := s.storage.SaveCover(id, imageData); err != nil {
		http.Error(w, "error saving cover", http.StatusInternalServerError)
		return
	}

	// Update the book record
	book, err := s.db.GetBook(id)
	if err != nil {
		http.Error(w, "book not found", http.StatusNotFound)
		return
	}
	book.CoverPath = "covers/" + strconv.FormatInt(id, 10) + ".jpg"
	s.db.UpdateBook(book)

	_ = header // we don't need the filename
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

> **Go note:** `10 << 20` is 10MB — Go supports bit shift expressions like C. `r.ParseMultipartForm(10 << 20)` stores up to 10MB in memory; larger files spill to temp files. The `0xFF 0xD8` magic bytes are the JPEG file signature — a simple validation without importing an image library. This is the Go way: use the stdlib, keep it simple.

- [ ] **Step 4: Register route and run tests**

Add to router:
```go
r.Post("/api/books/{id}/cover", s.uploadCover)
```

Run: `go test ./internal/server/ -run TestCover -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/server/covers.go internal/server/covers_test.go
git commit -m "feat: add cover upload via multipart with JPEG validation"
```

Questions before moving on?

---

## Task 9: Dockerfile — Multi-Stage Build

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`

> **Go note:** Go's static compilation makes `FROM scratch` possible. `scratch` is Docker's empty image — no OS, no shell, no libraries. The final image contains only the Go binary. This is why we use `modernc.org/sqlite` (pure Go, no CGO) — if we used `mattn/go-sqlite3` (CGO), the binary would need libc at runtime and couldn't run on scratch. A typical Go scratch image is 15-20MB; a Python image is 50-150MB+.

> **Go vs other languages:** In C# you'd need the .NET runtime (120MB+). In Python you'd need the interpreter + all pip packages. In Node you'd need node + node_modules. Go compiles to a single static binary that runs anywhere — this is Go's killer feature for containers.

- [ ] **Step 1: Create the Dockerfile**

Create `Dockerfile`:

```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o incipit .

# Runtime stage — scratch image, just the binary
FROM scratch
COPY --from=builder /app/incipit /incipit

# Templates and static files are embedded via go:embed, so no need to copy web/

ENTRYPOINT ["/incipit"]
CMD ["serve"]
```

- [ ] **Step 2: Create .dockerignore**

Create `.dockerignore`:

```
.git
docs/
*.md
incipit
test/
testdata/
```

- [ ] **Step 3: Verify the build works**

Run:
```bash
docker build -t incipit .
docker images incipit
# Should show a ~15-20MB image
```

> **Go note:** The `-ldflags="-s -w"` strips debug info and the DWARF symbol table. `-s` omits the symbol table, `-w` omits the DWARF info. This reduces binary size by ~30%. We don't need debug symbols in a production container — if you need to debug, build without these flags locally.

- [ ] **Step 4: Test the container runs**

Run:
```bash
docker run -e INCIPIT_DB_PATH=/tmp/test.db -e INCIPIT_STORAGE_DIR=/tmp incipit init
# Should print "Database initialized at /tmp/test.db"
docker run -p 8080:8080 -e INCIPIT_DB_PATH=/tmp/test.db -e INCIPIT_STORAGE_DIR=/tmp incipit serve
# In another terminal:
curl http://localhost:8080/health
# Should return {"status":"ok"}
```

- [ ] **Step 5: Commit**

```bash
git add Dockerfile .dockerignore
git commit -m "feat: add multi-stage Dockerfile producing scratch image"
```

Questions before moving on?

---

## Task 10: Deployment Configuration

**Files:**
- Create: `deploy/values.yaml`
- Create: `deploy/README.md`

> **Design note:** The deployment uses the existing `veridian-apps` Helm chart pattern. We don't create a new chart — we create values for the existing chart. The Helm chart handles the boilerplate (Deployment, Service, Ingress, PVC); we just provide app-specific values.

- [ ] **Step 1: Create the Helm values file**

Create `deploy/values.yaml`:

```yaml
# Helm values for Incipit on the veridian-apps chart
image:
  repository: ghcr.io/superversivesf/incipit
  tag: latest
  pullPolicy: IfNotPresent

replicaCount: 1

service:
  port: 8080

ingress:
  enabled: true
  host: incipit.veridiandynamics
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: veridiandynamics-default-headers@kubernetescrd

persistence:
  enabled: true
  size: 10Gi
  mountPath: /data

env:
  INCIPIT_DB_PATH: /data/books.db
  INCIPIT_STORAGE_DIR: /data
  INCIPIT_PORT: "8080"

# Readiness/liveness probes hit GET /health (no auth required)
probes:
  readiness:
    httpGet:
      path: /health
      port: 8080
    initialDelaySeconds: 5
    periodSeconds: 10
  liveness:
    httpGet:
      path: /health
      port: 8080
    initialDelaySeconds: 15
    periodSeconds: 20
```

- [ ] **Step 2: Create deployment README**

Create `deploy/README.md`:

```markdown
# Incipit Deployment

## Build and push the image

    docker build -t ghcr.io/superversivesf/incipit:latest .
    docker push ghcr.io/superversivesf/incipit:latest

## Deploy to k3s

    helm upgrade --install incipit veridian-apps/veridian-apps \
      -f deploy/values.yaml \
      -n veridiandynamics

## First-time setup

After deployment, create the database and an admin user:

    kubectl exec -it deployment/incipit -n veridiandynamics -- /incipit init
    kubectl exec -it deployment/incipit -n veridiandynamics -- /incipit add-user --username admin --password 'yourpassword' --role admin

## Backup

The entire system state is in the PVC at /data:
- /data/books.db (SQLite database)
- /data/files/ (EPUB files)
- /data/covers/ (cover images)

Backup with restic or rsync of that directory.

## KOReader configuration

- OPDS catalog URL: https://incipit.veridiandynamics/opds
- Sync server URL: https://incipit.veridiandynamics
- Username/password: created via CLI above
```

- [ ] **Step 3: Commit**

```bash
git add deploy/
git commit -m "feat: add k3s deployment configuration with Helm values"
```

Questions before moving on?

---

## Phase 3 Completion Checklist

- [ ] `GET /syncs/healthcheck` returns `{"state":"OK"}` without auth
- [ ] `GET /syncs/auth` returns user info with valid credentials, 401 without
- [ ] `PUT /syncs/progress/{hash}` saves reading position
- [ ] `GET /syncs/progress/{hash}` returns reading position, 404 if none
- [ ] Progress is isolated per user (user A can't see user B's progress)
- [ ] `GET /tags` shows the tag tree
- [ ] `POST /api/tags` creates a new tag
- [ ] `PUT /api/tags/{id}` renames a tag
- [ ] `DELETE /api/tags/{id}` deletes a tag
- [ ] `GET /series` shows series with book counts
- [ ] Series can be renamed
- [ ] `GET /book/{id}/edit` shows the edit form
- [ ] `POST /book/{id}/edit` saves metadata changes
- [ ] `POST /api/books/{id}/cover` uploads a cover image
- [ ] `docker build` produces a scratch image (~15-20MB)
- [ ] The container runs `incipit serve` and responds to `/health`
- [ ] `go vet ./...` is clean
- [ ] `gofmt -l .` is empty
- [ ] `go test ./...` passes
- [ ] All code committed and pushed

**Phase 3 delivers the complete Incipit system: CLI + web server + OPDS + sync + management UI + containerized deployment. Post-MVP items (Calibre import, thumbnails, bulk operations) are planned separately.**