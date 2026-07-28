# Incipit Phase 1: CLI Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Incipit CLI core — a usable command-line ebook library tool with DB, EPUB parsing, metadata lookup, and book management.

**Architecture:** Sequential by phase. Phase 1 builds `internal/{db,epub,lookup,models,storage,search,config}` packages and CLI subcommands wired through `main.go`. All app code under `internal/`, `models` as shared type hub, `db` owns all SQL, pure-Go SQLite via `modernc.org/sqlite`.

**Tech Stack:** Go 1.22, `modernc.org/sqlite` (pure-Go SQLite, no CGO), `golang.org/x/crypto/bcrypt`, Go stdlib only.

## Global Constraints

- Go 1.22, module path `github.com/jason/incipit`
- Pure-Go SQLite via `modernc.org/sqlite` — no CGO. Build with `CGO_ENABLED=0`.
- Dependency allowlist: `modernc.org/sqlite`, `golang.org/x/crypto/bcrypt`, plus Go stdlib only.
- All app code under `internal/`.
- Password hashing: server stores `bcrypt(MD5(password))`. CLI `add-user` takes plaintext → MD5 → bcrypt → store.
- `models` depends on nothing; `db`/`epub`/`lookup`/`search`/`storage` depend only on `models`.
- `db` owns all SQL. Typed methods take/return `models` structs.
- `epub` is pure parsing — no DB, no HTTP.
- `lookup` does not import `db`. Caching handled by caller.
- Migrations: versioned, embedded SQL via `embed.FS` in `internal/db/migrations/`.
- PRAGMAs on open: `journal_mode=WAL`, `foreign_keys=ON`.
- Testing: stdlib `testing`, real on-disk SQLite in `t.TempDir()`, fixtures in `testdata/`. No network in tests.
- Quality gates: `go vet ./...` clean, `gofmt -l .` empty, `go test ./...` passing.

## File Structure

```
incipit/
├── go.mod
├── go.sum
├── main.go                          # Subcommand dispatch
├── internal/
│   ├── config/
│   │   └── config.go                # Env var loading
│   ├── models/
│   │   ├── models.go                # Book, Tag, User, ReadingProgress, Metadata, LookupResult
│   │   ├── sort.go                  # SortTitle function
│   │   └── merge.go                 # MergeMetadata function
│   ├── db/
│   │   ├── db.go                    # Open, Close, WithTx, DB wrapper
│   │   ├── migrate.go               # Migrate using embedded SQL
│   │   ├── migrations/
│   │   │   └── 001_init.sql         # Full schema
│   │   ├── users.go                 # User CRUD
│   │   ├── books.go                 # Book CRUD
│   │   └── cache.go                 # Metadata cache get/put
│   ├── epub/
│   │   ├── epub.go                   # Parse, ParseOPF
│   │   └── testdata/
│   │       └── (fixture EPUBs)
│   ├── lookup/
│   │   ├── lookup.go                # Lookup orchestrator, Client interface
│   │   ├── openlibrary.go           # OL client
│   │   ├── googlebooks.go            # GB client
│   │   ├── merge.go                 # Merge(ol, gb) function
│   │   └── testdata/
│   │       └── (fixture JSON)
│   ├── storage/
│   │   └── storage.go               # SaveBookFile, SaveCover, HashFile, paths
│   └── search/
│       ├── search.go                # Searcher interface
│       └── like.go                  # LikeSearcher
```

---

## Task 1: Go Module + Project Skeleton

**Files:**
- Create: `go.mod`, `main.go`

> **Go note:** A Go module is the unit of distribution — like a npm package or a NuGet project, but simpler. `go mod init` creates `go.mod` which tracks the module path and dependencies. Unlike Node.js, Go compiles to a single static binary — no `node_modules`, no runtime dependency resolution.

> **Go note:** The `internal/` directory is a Go convention that prevents other packages from importing your code. Anything under `internal/` can only be imported by packages within the same module. It's like C#'s `internal` access modifier but at the directory level. We put all app code here so external packages can't reach into our internals.

- [ ] **Step 1: Initialize the Go module and create the directory structure**

```bash
cd ~/Repos/incipit
go mod init github.com/jason/incipit
mkdir -p internal/{config,models,db/migrations,epub/testdata,lookup/testdata,storage,search}
```

- [ ] **Step 2: Write a minimal `main.go`**

```go
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: incipit <command> [args]")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		fmt.Println("init: not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}
```

> **Go note:** `package main` is special — it's the entry point. Go's `func main()` is like C's `main()` but takes no argc/argv (you read `os.Args` manually). `os.Args[0]` is the program name, `os.Args[1]` is the first argument — same as C, different from Python where `sys.argv[0]` is the script name.

> **Go vs other languages:** Go has no exceptions. `os.Exit(2)` is how you exit with an error code — there's no `try/catch`, no `System.Environment.Exit()`. Error codes are the convention for CLI tools. You'll see this pattern everywhere in Go: errors are values, not control flow.

- [ ] **Step 3: Verify it builds and runs**

Run: `go build -o incipit . && ./incipit init`
Expected: prints `init: not yet implemented`

Run: `./incipit`
Expected: prints `usage: incipit <command> [args]` and exits with code 2

- [ ] **Step 4: Commit**

```bash
git add go.mod main.go
git commit -m "feat: initialize Go module and main.go skeleton"
```

Questions before moving on?

---

## Task 2: Models — Book Struct and SortTitle

**Files:**
- Create: `internal/models/models.go`
- Create: `internal/models/sort.go`
- Test: `internal/models/sort_test.go`

**Interfaces:**
- Produces: `models.Book`, `models.Tag`, `models.User`, `models.ReadingProgress`, `models.Metadata`, `models.LookupResult`, `models.SortTitle(string) string`

> **Go note:** Go structs are like C structs — plain data, no methods attached by default. Unlike C# classes, there's no inheritance, no constructors, no properties. You define a struct, then optionally attach methods to it by writing `func (s MyStruct) MethodName()`. The `(s MyStruct)` part is the "receiver" — it binds the method to the type.

> **Go visibility:** In Go, visibility is determined by capitalization. `Title` (capital T) is exported — other packages can see it. `title` (lowercase t) is private. This replaces `public`/`private` keywords from C#/Java. Struct names, field names, function names — all follow this rule. It takes getting used to but it's remarkably effective.

- [ ] **Step 1: Write the failing test for SortTitle**

Create `internal/models/sort_test.go`:

```go
package models

import "testing"

func TestSortTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Expanse", "Expanse, The"},
		{"A Brief History of Time", "Brief History of Time, A"},
		{"An Ocean of Night", "Ocean of Night, An"},
		{"Leviathan Wakes", "Leviathan Wakes"},
		{"", ""},
	}
	for _, tt := range tests {
		got := SortTitle(tt.input)
		if got != tt.expected {
			t.Errorf("SortTitle(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
```

> **Go note:** This is "table-driven testing" — the idiomatic Go testing pattern. You define a slice of test cases (structs with input + expected output), then loop over them. It's like `[Theory]`/`[InlineData]` in C# xUnit or `pytest.mark.parametrize`, but with zero framework — just stdlib `testing`.

> **Go note:** `t.Errorf` is like `assert.Equal` but it doesn't stop the test — it records the failure and continues. Use `t.Fatalf` if you need to stop immediately (e.g., a setup step failed). Most assertions in Go are manual: `if got != want { t.Errorf(...) }`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestSortTitle -v`
Expected: FAIL with `undefined: SortTitle`

- [ ] **Step 3: Write SortTitle implementation**

Create `internal/models/sort.go`:

```go
package models

import "strings"

var articles = []string{"The ", "A ", "An "}

func SortTitle(title string) string {
	for _, article := range articles {
		if strings.HasPrefix(title, article) {
			return title[len(article):] + ", " + strings.TrimSpace(article)
		}
	}
	return title
}
```

> **Go note:** `strings` is from the stdlib — no import needed beyond `import "strings"`. Unlike Python where string methods are on the object (`s.startswith()`), Go uses functions: `strings.HasPrefix(s, prefix)`. This is because Go strings are read-only byte slices, not objects.

> **Go note:** `range` over a slice gives you `(index, value)`. Using `_` for the index means "I don't need it" — Go requires you to acknowledge every return value, it won't let you silently ignore them. This is stricter than Python's `for x in list`.

- [ ] **Step 4: Write the model type definitions**

Create `internal/models/models.go`:

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
	Description string
	Publisher   string
	Published   string
	Pages       int
	Rating      float64
	CoverPath   string
	FilePath    string
	FileHash    string
	FileSize    int64
	Added       string
	Updated     string
}

type Tag struct {
	ID       int64
	Name     string
	ParentID *int64
}

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	Created      string
}

type ReadingProgress struct {
	BookID     *int64
	UserID     int64
	Percentage float64
	Progress   string
	Device     string
	Updated    string
}

type Metadata struct {
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
```

> **Go note:** `*int64` (pointer) is used for nullable fields. SQLite allows NULL, but Go has no null — `int64` is always a value. A pointer (`*int64`) can be `nil` to represent "no value." This is the Go equivalent of `int?` in C# or `Optional<int>` in Java. In Task 3, we'll use `sql.NullInt64` for DB reads, but the model layer uses pointers for clarity.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/models/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/models/
git commit -m "feat: add models package with Book struct and SortTitle"
```

Questions before moving on?

---

## Task 3: DB — Open, PRAGMAs, and Migrate

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/migrate.go`
- Create: `internal/db/migrations/001_init.sql`
- Test: `internal/db/db_test.go`

**Interfaces:**
- Consumes: nothing (standalone)
- Produces: `db.Open(path string) (*DB, error)`, `db.DB.Close() error`, `db.DB.Migrate() error`

> **Go note:** `database/sql` is Go's database abstraction layer — like ADO.NET's `DbConnection` or Python's DB-API. It provides a connection pool, prepared statements, and transactions. You register a driver (here, `modernc.org/sqlite`) and `sql.Open` returns a `*sql.DB` which is a pool, not a single connection.

> **CGO note:** `modernc.org/sqlite` is a pure-Go port of SQLite. The standard `mattn/go-sqlite3` wraps the C library and requires CGO. We use the pure-Go version so we can cross-compile and build `FROM scratch` Docker images. The tradeoff is slightly slower performance, which is fine for our use case.

- [ ] **Step 1: Add the SQLite dependency**

```bash
go get modernc.org/sqlite
```

- [ ] **Step 2: Write the failing test for Open + Migrate**

Create `internal/db/db_test.go`:

```go
package db

import (
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var name string
	err = d.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='books'").Scan(&name)
	if err != nil {
		t.Fatalf("books table not found after migrate: %v", err)
	}
	if name != "books" {
		t.Errorf("expected 'books', got %q", name)
	}
}
```

> **Go note:** `t.TempDir()` creates a unique temporary directory for the test and automatically cleans it up when the test finishes. This is like pytest's `tmp_path` fixture — no manual cleanup needed. Each test gets its own fresh database.

> **Go note:** `defer d.Close()` schedules `Close()` to run when the function returns, regardless of how it returns (normal or error). This is Go's replacement for `finally` blocks in C#/Java/Python. `defer` is stack-based — multiple defers run in LIFO order.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestOpenAndMigrate -v`
Expected: FAIL with `undefined: Open`

- [ ] **Step 4: Write the migration SQL**

Create `internal/db/migrations/001_init.sql`:

```sql
CREATE TABLE IF NOT EXISTS books (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    title         TEXT NOT NULL,
    title_sort    TEXT,
    author        TEXT NOT NULL,
    author_sort   TEXT,
    series        TEXT,
    series_index  REAL,
    isbn          TEXT,
    description   TEXT,
    publisher     TEXT,
    published     TEXT,
    pages         INTEGER,
    rating        REAL,
    cover_path    TEXT,
    file_path     TEXT NOT NULL,
    file_hash     TEXT,
    file_size     INTEGER,
    added         TEXT DEFAULT (datetime('now')),
    updated       TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tags (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    parent_id INTEGER,
    FOREIGN KEY (parent_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS book_tags (
    book_id INTEGER NOT NULL,
    tag_id  INTEGER NOT NULL,
    PRIMARY KEY (book_id, tag_id),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role          TEXT DEFAULT 'user',
    created       TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS reading_progress (
    book_id     INTEGER,
    user_id     INTEGER NOT NULL,
    percentage  REAL,
    progress    TEXT,
    device      TEXT,
    updated     TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (book_id, user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS metadata_cache (
    isbn        TEXT,
    title       TEXT,
    author      TEXT,
    source      TEXT,
    response    TEXT,
    cached_at   TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (isbn, source)
);

CREATE INDEX IF NOT EXISTS idx_books_author ON books(author);
CREATE INDEX IF NOT EXISTS idx_books_series ON books(series);
CREATE INDEX IF NOT EXISTS idx_books_title_sort ON books(title_sort);
CREATE INDEX IF NOT EXISTS idx_books_added ON books(added);
CREATE INDEX IF NOT EXISTS idx_book_tags_tag ON book_tags(tag_id);
CREATE INDEX IF NOT EXISTS idx_reading_progress_book ON reading_progress(book_id);
CREATE INDEX IF NOT EXISTS idx_reading_progress_user ON reading_progress(user_id);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT DEFAULT (datetime('now'))
);
```

> **Go note:** `embed.FS` is a Go 1.16+ feature that embeds files into the binary at compile time. The `//go:embed` directive tells the compiler to include the specified files. This is how we ship SQL migrations and (later) HTML templates inside the binary — no external files needed at runtime. It's like Go's answer to .NET's embedded resources but much simpler.

> **Design note:** `reading_progress.book_id` is nullable here (no `NOT NULL`, no FK to books). This is the deliberate spec deviation — it allows KOReader sync by document hash even when the book isn't in the library. See design doc Section 6.

- [ ] **Step 5: Write the DB wrapper and Open**

Create `internal/db/db.go`:

```go
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("setting pragma %q: %w", p, err)
		}
	}

	return &DB{db: sqlDB}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}
```

> **Go note:** The blank import `_ "modernc.org/sqlite"` registers the SQLite driver with `database/sql` without exposing any names. The driver registers itself via `init()` — a special function that runs automatically when a package is imported. This is Go's plugin/registration pattern, replacing C#'s reflection-based driver discovery.

> **Go note:** `fmt.Errorf("...: %w", err)` wraps an error with context. The `%w` verb (not `%s` or `%v`) preserves the error chain so callers can use `errors.Is()` and `errors.As()` to inspect it. This is Go's approach to error context — unlike Python's `raise ... from ...`, it's done at the point of wrapping, not the point of raising.

> **Go vs other languages:** Go has no constructors. `Open(path string) (*DB, error)` is a plain function (a "constructor function" by convention, but the language doesn't enforce it). Unlike C# `new SqlConnection()`, there's no `new` keyword — `&DB{db: sqlDB}` creates a struct on the heap and takes its address. The `&` is Go's address-of operator, same as C.

- [ ] **Step 6: Write the Migrate function**

Create `internal/db/migrate.go`:

```go
package db

import (
	"embed"
	"fmt"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func (d *DB) Migrate() error {
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			continue
		}

		var applied int
		err := d.db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied)
		if err != nil {
			return fmt.Errorf("checking migration %d: %w", version, err)
		}
		if applied > 0 {
			continue
		}

		sqlBytes, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading migration file %s: %w", entry.Name(), err)
		}

		tx, err := d.db.Begin()
		if err != nil {
			return fmt.Errorf("beginning transaction for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(string(sqlBytes)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %d: %w", version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %d: %w", version, err)
		}
	}

	return nil
}
```

> **Go note:** `Begin()` starts a transaction. Go's `database/sql` requires manual `Commit()` or `Rollback()` — there's no `using` block or context manager like C#'s `transaction.Scope` or Python's `with`. The pattern is: `tx, err := db.Begin()` → do work → `tx.Commit()` or `tx.Rollback()`. Always roll back on error.

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestOpenAndMigrate -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/db/ go.mod go.sum
git commit -m "feat: add db package with Open, PRAGMAs, and embedded migrations"
```

Questions before moving on?

---

## Task 4: DB — User CRUD

**Files:**
- Create: `internal/db/users.go`
- Test: `internal/db/users_test.go`

**Interfaces:**
- Consumes: `db.DB` (from Task 3)
- Produces: `db.DB.CreateUser(username, passwordHash, role string) (int64, error)`, `db.DB.GetUser(username string) (*models.User, error)`, `db.DB.ListUsers() ([]models.User, error)`, `db.DB.DeleteUser(username string) error`

> **Go note:** Go methods are defined on a receiver type. `func (d *DB) CreateUser(...)` means `CreateUser` is a method on `*DB`. The receiver `(d *DB)` is like `this` in C++/C# but explicit — you choose the name. Using `d` (or `s` for server, `r` for router) is the convention. Pointer receiver (`*DB`) means the method can modify the struct and avoids copying.

- [ ] **Step 1: Write the failing tests for User CRUD**

Create `internal/db/users_test.go`:

```go
package db

import (
	"testing"
)

func TestCreateAndGetUser(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	id, err := d.CreateUser("alice", "hashedpassword123", "admin")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}

	user, err := d.GetUser("alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if user.Username != "alice" {
		t.Errorf("expected username 'alice', got %q", user.Username)
	}
	if user.PasswordHash != "hashedpassword123" {
		t.Errorf("expected hash 'hashedpassword123', got %q", user.PasswordHash)
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got %q", user.Role)
	}
}

func TestGetUserNotFound(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	_, err = d.GetUser("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
}

func TestCreateUserDuplicateUpdatesPassword(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("bob", "oldhash", "user")
	id2, err := d.CreateUser("bob", "newhash", "user")
	if err != nil {
		t.Fatalf("CreateUser (update) failed: %v", err)
	}

	user, _ := d.GetUser("bob")
	if user.PasswordHash != "newhash" {
		t.Errorf("expected updated hash 'newhash', got %q", user.PasswordHash)
	}

	_ = id2
}

func TestListUsers(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("alice", "h1", "admin")
	d.CreateUser("bob", "h2", "user")

	users, err := d.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestDeleteUser(t *testing.T) {
	d, err := Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	d.CreateUser("alice", "h1", "admin")
	d.DeleteUser("alice")

	_, err = d.GetUser("alice")
	if err == nil {
		t.Fatal("expected error after deletion, got nil")
	}
}
```

> **Go note:** We have five test functions in one file. Go runs each function as a separate test — no test class needed. Unlike C#/xUnit where tests are methods on a class, Go tests are just functions matching `func TestXxx(t *testing.T)`. Each function gets its own fresh database because we call `Open` with `t.TempDir()` each time.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run TestCreateAndGetUser -v`
Expected: FAIL with `undefined: d.CreateUser`

- [ ] **Step 3: Write the User CRUD implementation**

Create `internal/db/users.go`:

```go
package db

import (
	"database/sql"
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) CreateUser(username, passwordHash, role string) (int64, error) {
	result, err := d.db.Exec(
		`INSERT INTO users (username, password_hash, role)
		 VALUES (?, ?, ?)
		 ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash`,
		username, passwordHash, role,
	)
	if err != nil {
		return 0, fmt.Errorf("creating/updating user %s: %w", username, err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting user ID: %w", err)
	}
	return id, nil
}

func (d *DB) GetUser(username string) (*models.User, error) {
	var u models.User
	err := d.db.QueryRow(
		`SELECT id, username, password_hash, role, created FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Created)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user %s not found: %w", username, err)
		}
		return nil, fmt.Errorf("getting user %s: %w", username, err)
	}
	return &u, nil
}

func (d *DB) ListUsers() ([]models.User, error) {
	rows, err := d.db.Query(
		`SELECT id, username, password_hash, role, created FROM users ORDER BY username`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.Created); err != nil {
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (d *DB) DeleteUser(username string) error {
	_, err := d.db.Exec(`DELETE FROM users WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("deleting user %s: %w", username, err)
	}
	return nil
}
```

> **Go note:** `QueryRow` is for queries returning at most one row — it's simpler than `Query` (no `rows.Close()` needed). `Scan` copies column values into the provided pointers. If no row matches, you get `sql.ErrNoRows` — Go's way of representing "no result" without null.

> **Go note:** `defer rows.Close()` is critical. If you forget to close rows, the database connection stays checked out from the pool. Go's `go vet` can catch some defer misses, but it's your responsibility. This is like forgetting to `Dispose()` in C# — except Go makes it easier with `defer`.

> **Go note:** `rows.Err()` after the loop returns any error that occurred during iteration. The loop `for rows.Next()` stops on error, but the error isn't available until you call `rows.Err()`. This is a common Go gotcha — always check `rows.Err()` after the loop.

> **Go vs other languages:** SQL uses `?` placeholders (SQLite style). MySQL also uses `?`. PostgreSQL uses `$1, $2`. This is driver-specific, not standardized by Go. Named parameters (`@name`) are available via `sql.Named` for complex queries.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/users.go internal/db/users_test.go
git commit -m "feat: add user CRUD to db package"
```

Questions before moving on?

---

## Task 5: Config — Load from Environment

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` struct, `config.Load() Config`

> **Go note:** `os.Getenv("KEY")` returns the value or an empty string if not set — it doesn't distinguish "set to empty" from "not set". For our defaults, that's fine: if the env var is empty, we use the default. This is simpler than Python's `os.environ.get("KEY", "default")` but less precise — `os.LookupEnv` is the precise version.

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Unsetenv("INCIPIT_DB_PATH")
	os.Unsetenv("INCIPIT_PORT")
	os.Unsetenv("INCIPIT_STORAGE_DIR")

	cfg := Load()

	if cfg.DBPath != "/data/books.db" {
		t.Errorf("expected default DBPath '/data/books.db', got %q", cfg.DBPath)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default Port '8080', got %q", cfg.Port)
	}
	if cfg.StorageDir != "/data" {
		t.Errorf("expected default StorageDir '/data', got %q", cfg.StorageDir)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("INCIPIT_DB_PATH", "/tmp/custom.db")
	t.Setenv("INCIPIT_PORT", "9090")
	t.Setenv("INCIPIT_STORAGE_DIR", "/custom/storage")

	cfg := Load()

	if cfg.DBPath != "/tmp/custom.db" {
		t.Errorf("expected DBPath '/tmp/custom.db', got %q", cfg.DBPath)
	}
	if cfg.Port != "9090" {
		t.Errorf("expected Port '9090', got %q", cfg.Port)
	}
	if cfg.StorageDir != "/custom/storage" {
		t.Errorf("expected StorageDir '/custom/storage', got %q", cfg.StorageDir)
	}
}
```

> **Go note:** `t.Setenv` sets an env var for the duration of the test and automatically restores the original value when the test ends. This is like `monkeypatch.setenv` in pytest but built into the testing package. Don't use `os.Setenv` directly in tests — `t.Setenv` is safer because it cleans up.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL with `undefined: Load`

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:

```go
package config

import "os"

type Config struct {
	DBPath     string
	Port       string
	StorageDir string
}

func Load() Config {
	return Config{
		DBPath:     envOrDefault("INCIPIT_DB_PATH", "/data/books.db"),
		Port:       envOrDefault("INCIPIT_PORT", "8080"),
		StorageDir: envOrDefault("INCIPIT_STORAGE_DIR", "/data"),
	}
}

func envOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with env var loading"
```

Questions before moving on?

---

## Task 6: main.go — init and add-user Commands

**Files:**
- Modify: `main.go`
- Create: `internal/db/users.go` (already exists — we use bcrypt here via main.go)

**Interfaces:**
- Consumes: `db.Open`, `db.Migrate`, `db.CreateUser`, `db.ListUsers`, `db.DeleteUser`, `config.Load`

> **Go note:** The `flag` package is Go's stdlib argument parser — like Python's `argparse` but more basic. You create a `FlagSet`, register flags, then parse. Unlike most languages, Go doesn't have built-in subcommand routing — you do it manually with a `switch` on `os.Args[1]`. Libraries like `cobra` exist, but stdlib is sufficient for our case.

> **Password hashing chain:** KOReader MD5-hashes the password before sending it to the server. The server stores `bcrypt(MD5(password))`. The CLI `add-user` takes plaintext → MD5-hash → bcrypt-hash → store. So when KOReader sends `MD5(password)` via basic auth, the server compares it against the stored `bcrypt(MD5(password))`.

- [ ] **Step 1: Add bcrypt dependency**

```bash
go get golang.org/x/crypto/bcrypt
```

- [ ] **Step 2: Update main.go with init, add-user, list-users, remove-user**

Rewrite `main.go`:

```go
package main

import (
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: incipit <command> [args]")
		fmt.Fprintln(os.Stderr, "commands: init, serve, parse, lookup, add, add-user, list-users, remove-user")
		os.Exit(2)
	}

	switch os.Args[1] {
	case "init":
		runInit()
	case "add-user":
		runAddUser(os.Args[2:])
	case "list-users":
		runListUsers()
	case "remove-user":
		runRemoveUser(os.Args[2:])
	case "serve":
		fmt.Println("serve: not yet implemented")
	case "parse":
		fmt.Println("parse: not yet implemented")
	case "lookup":
		fmt.Println("lookup: not yet implemented")
	case "add":
		fmt.Println("add: not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(2)
	}
}

func runInit() {
	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Database initialized at %s\n", cfg.DBPath)
}

func runAddUser(args []string) {
	fs := flag.NewFlagSet("add-user", flag.ExitOnError)
	username := fs.String("username", "", "username")
	password := fs.String("password", "", "password (plaintext)")
	role := fs.String("role", "user", "role (user or admin)")
	fs.Parse(args)

	if *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit add-user --username X --password Y [--role admin]")
		os.Exit(2)
	}

	hash, err := hashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error hashing password: %v\n", err)
		os.Exit(1)
	}

	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.Migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "error running migrations: %v\n", err)
		os.Exit(1)
	}

	if _, err := d.CreateUser(*username, hash, *role); err != nil {
		fmt.Fprintf(os.Stderr, "error creating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %s created (role: %s)\n", *username, *role)
}

func runListUsers() {
	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	users, err := d.ListUsers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing users: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-20s %-10s %s\n", "USERNAME", "ROLE", "ID")
	for _, u := range users {
		fmt.Printf("%-20s %-10s %d\n", u.Username, u.Role, u.ID)
	}
}

func runRemoveUser(args []string) {
	fs := flag.NewFlagSet("remove-user", flag.ExitOnError)
	username := fs.String("username", "", "username to remove")
	fs.Parse(args)

	if *username == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit remove-user --username X")
		os.Exit(2)
	}

	cfg := config.Load()
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()

	if err := d.DeleteUser(*username); err != nil {
		fmt.Fprintf(os.Stderr, "error deleting user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User %s removed\n", *username)
}

func hashPassword(plaintext string) (string, error) {
	md5sum := md5.Sum([]byte(plaintext))
	md5hex := hex.EncodeToString(md5sum[:])

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(md5hex), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hashing: %w", err)
	}

	return string(bcryptHash), nil
}

// suppress unused import until serve command is implemented
var _ = sql.ErrNoRows
var _ = strconv.Itoa
```

> **Go note:** `flag.NewFlagSet` creates a parser for a subcommand. `fs.String("username", "", "description")` registers a string flag and returns a `*string` (pointer to the value). You dereference with `*username` to get the value. This pointer-based API is different from most flag parsers — it's because Go doesn't have out parameters or tuples.

> **Go note:** `md5.Sum` returns a `[16]byte` array (fixed-size, unlike slices). We convert it to a hex string via `hex.EncodeToString(md5sum[:])` — the `[:]` slices the array into a `[]byte`. This array-to-slice conversion is unique to Go — you can't do this with C arrays without casting.

> **Go note:** The `var _ = sql.ErrNoRows` at the bottom suppresses "unused import" errors. Go is strict — if you import a package but don't use it, compilation fails. This is aggressive compared to Python (unused imports are a lint warning) or C# (unused usings are a warning). The blank identifier `_` is Go's "I acknowledge this but don't use it" escape hatch.

- [ ] **Step 3: Verify init and add-user work end-to-end**

```bash
# Test init
INCIPIT_DB_PATH=/tmp/incipit-test.db go run main.go init
# Should print: "Database initialized at /tmp/incipit-test.db"

# Test add-user
INCIPIT_DB_PATH=/tmp/incipit-test.db go run main.go add-user --username admin --password secret --role admin
# Should print: "User admin created (role: admin)"

# Test list-users
INCIPIT_DB_PATH=/tmp/incipit-test.db go run main.go list-users
# Should print a table with the admin user

# Test remove-user
INCIPIT_DB_PATH=/tmp/incipit-test.db go run main.go remove-user --username admin
# Should print: "User admin removed"

rm /tmp/incipit-test.db
```

- [ ] **Step 4: Run quality gates**

Run: `go vet ./... && gofmt -l . && go test ./...`
Expected: vet clean, gofmt empty, all tests pass

- [ ] **Step 5: Commit**

```bash
git add main.go go.mod go.sum
git commit -m "feat: add init, add-user, list-users, remove-user CLI commands"
```

Questions before moving on?

---

## Task 7: EPUB Parser — Parse and ParseOPF

**Files:**
- Create: `internal/epub/epub.go`
- Test: `internal/epub/epub_test.go`

**Interfaces:**
- Consumes: `models.Metadata`
- Produces: `epub.Parse(path string) (*models.Metadata, error)`, `epub.ParseOPF(r io.Reader) (*models.Metadata, error)`

> **Go note:** EPUB is a ZIP file containing XML. Go's stdlib has both `archive/zip` and `encoding/xml` — no third-party libraries needed. This is a major advantage of Go for file processing — the stdlib covers a lot of ground. In Python you'd use `zipfile` + `xml.etree`, in C# you'd use `System.IO.Compression` + `System.Xml`.

> **Go XML namespaces:** Go's `encoding/xml` handles namespaces via struct tags. The field `XMLName xml.Name `xml:"http://purl.org/dc/elements/1.1/ title"`` means "this field maps to the `<title>` element in the Dublin Core namespace." Go's XML is not as slick as its JSON (no `json.Marshal` equivalent for arbitrary maps), but it works well for structured documents.

- [ ] **Step 1: Write the failing test**

Create `internal/epub/epub_test.go`:

```go
package epub

import (
	"archive/zip"
	"bytes"
	"testing"
)

// createTestEPUB builds a minimal EPUB in memory for testing.
// An EPUB is a ZIP containing META-INF/container.xml and an OPF file.
func createTestEPUB(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	// container.xml points to the OPF file
	containerXML := `<?xml version="1.0"?>
<container version="1.0">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`

	// OPF with Dublin Core metadata
	opfXML := `<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/"
         xmlns:opf="http://www.idpf.org/2007/opf/"
         version="3.0">
  <metadata>
    <dc:title>Leviathan Wakes</dc:title>
    <dc:creator opf:role="aut">James S. A. Corey</dc:creator>
    <dc:identifier opf:scheme="ISBN">9780316129084</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>Orbit Books</dc:publisher>
    <dc:date>2011-06-15</dc:date>
  </metadata>
</package>`

	files := map[string]string{
		"META-INF/container.xml": containerXML,
		"OEBPS/content.opf":      opfXML,
	}
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("creating zip entry %s: %v", name, err)
		}
		f.Write([]byte(content))
	}
	w.Close()
	return buf.Bytes()
}

func TestParseFromBytes(t *testing.T) {
	epubBytes := createTestEPUB(t)

	// Write to a temp file for Parse
	tmpFile := t.TempDir() + "/test.epub"
	if err := osWriteFile(tmpFile, epubBytes); err != nil {
		t.Fatalf("writing temp epub: %v", err)
	}

	meta, err := Parse(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if meta.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", meta.Title)
	}
	if meta.Creator != "James S. A. Corey" {
		t.Errorf("expected creator 'James S. A. Corey', got %q", meta.Creator)
	}
	if meta.Identifier != "9780316129084" {
		t.Errorf("expected identifier '9780316129084', got %q", meta.Identifier)
	}
	if meta.Language != "en" {
		t.Errorf("expected language 'en', got %q", meta.Language)
	}
	if meta.Publisher != "Orbit Books" {
		t.Errorf("expected publisher 'Orbit Books', got %q", meta.Publisher)
	}
	if meta.Date != "2011-06-15" {
		t.Errorf("expected date '2011-06-15', got %q", meta.Date)
	}
}

func TestParseOPFFromReader(t *testing.T) {
	opfXML := `<?xml version="1.0"?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/"
         xmlns:opf="http://www.idpf.org/2007/opf/"
         version="3.0">
  <metadata>
    <dc:title>Test Book</dc:title>
    <dc:creator opf:role="aut">Test Author</dc:creator>
    <dc:identifier opf:scheme="ISBN">1234567890</dc:identifier>
  </metadata>
</package>`

	meta, err := ParseOPF(bytes.NewReader([]byte(opfXML)))
	if err != nil {
		t.Fatalf("ParseOPF failed: %v", err)
	}
	if meta.Title != "Test Book" {
		t.Errorf("expected title 'Test Book', got %q", meta.Title)
	}
	if meta.Creator != "Test Author" {
		t.Errorf("expected creator 'Test Author', got %q", meta.Creator)
	}
}
```

- [ ] **Step 2: Add the helper for writing files (used by the test)**

Add at the top of `epub_test.go` (after the imports):

```go
import "os"

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/epub/ -v`
Expected: FAIL with `undefined: Parse`

- [ ] **Step 4: Write the EPUB parser**

Create `internal/epub/epub.go`:

```go
package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jason/incipit/internal/models"
)

// container.xml structure
type container struct {
	XMLName   xml.Name `xml:"container"`
	Rootfiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

// OPF structure
type opfPackage struct {
	XMLName  xml.Name `xml:"package"`
	Metadata struct {
		Title       string `xml:"http://purl.org/dc/elements/1.1/ title"`
		Creators    []struct {
			Text string `xml:",chardata"`
			Role string `xml:"http://www.idpf.org/2007/opf/ role,attr"`
		} `xml:"http://purl.org/dc/elements/1.1/ creator"`
		Identifiers []struct {
			Text   string `xml:",chardata"`
			Scheme string `xml:"http://www.idpf.org/2007/opf/ scheme,attr"`
		} `xml:"http://purl.org/dc/elements/1.1/ identifier"`
		Language   string `xml:"http://purl.org/dc/elements/1.1/ language"`
		Publisher  string `xml:"http://purl.org/dc/elements/1.1/ publisher"`
		Date       string `xml:"http://purl.org/dc/elements/1.1/ date"`
	} `xml:"metadata"`
}

func Parse(path string) (*models.Metadata, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("opening epub zip: %w", err)
	}
	defer r.Close()

	var opfPath string
	for _, f := range r.File {
		if f.Name == "META-INF/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening container.xml: %w", err)
			}
			var c container
			if err := xml.NewDecoder(rc).Decode(&c); err != nil {
				rc.Close()
				return nil, fmt.Errorf("parsing container.xml: %w", err)
			}
			rc.Close()
			if len(c.Rootfiles) > 0 {
				opfPath = c.Rootfiles[0].FullPath
			}
			break
		}
	}

	if opfPath == "" {
		return nil, fmt.Errorf("no OPF path found in container.xml")
	}

	for _, f := range r.File {
		if f.Name == opfPath {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening OPF file: %w", err)
			}
			defer rc.Close()
			return ParseOPF(rc)
		}
	}

	return nil, fmt.Errorf("OPF file %q not found in epub", opfPath)
}

func ParseOPF(r io.Reader) (*models.Metadata, error) {
	var pkg opfPackage
	if err := xml.NewDecoder(r).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("parsing OPF: %w", err)
	}

	meta := &models.Metadata{
		Title:     pkg.Metadata.Title,
		Language:  pkg.Metadata.Language,
		Publisher: pkg.Metadata.Publisher,
		Date:      pkg.Metadata.Date,
	}

	for _, creator := range pkg.Metadata.Creators {
		if creator.Role == "aut" || (creator.Role == "" && meta.Creator == "") {
			if meta.Creator != "" {
				meta.Creator += ", "
			}
			meta.Creator += creator.Text
		}
	}

	for _, id := range pkg.Metadata.Identifiers {
		isbn := extractISBN(id.Text, id.Scheme)
		if isbn != "" {
			meta.Identifier = isbn
			break
		}
	}

	return meta, nil
}

func extractISBN(text, scheme string) string {
	if scheme == "ISBN" {
		return normalizeISBN(text)
	}
	text = strings.TrimPrefix(text, "urn:isbn:")
	return normalizeISBN(text)
}

func normalizeISBN(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

> **Go note:** Struct tags (`xml:"..."`) are metadata attached to struct fields. They're like C# attributes (`[XmlAttribute]`) but at the field level. The tag `xml:"http://purl.org/dc/elements/1.1/ title"` means "map this field to the `<title>` element in the Dublin Core namespace." The `chardata` directive captures text content.

> **Go note:** `xml.NewDecoder(r).Decode(&pkg)` is the streaming XML parser. Unlike `xml.Unmarshal` which reads the whole byte slice, `Decode` reads from a stream. Both populate a struct from XML. Go's XML unmarshaling is struct-based — you define Go structs that mirror the XML structure and the decoder fills them.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/epub/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/epub/
git commit -m "feat: add epub parser with Parse and ParseOPF"
```

Questions before moving on?

---

## Task 8: main.go — parse Command

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `epub.Parse`, `models.Metadata`

- [ ] **Step 1: Update the `parse` case in main.go**

In `main.go`, replace the `parse` case:

```go
	case "parse":
		runParse(os.Args[2:])
```

And add the function:

```go
func runParse(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: incipit parse <path>")
		os.Exit(2)
	}

	meta, err := epub.Parse(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing epub: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Title:      %s\n", meta.Title)
	fmt.Printf("Creator:    %s\n", meta.Creator)
	fmt.Printf("Identifier: %s\n", meta.Identifier)
	fmt.Printf("Language:   %s\n", meta.Language)
	fmt.Printf("Publisher:  %s\n", meta.Publisher)
	fmt.Printf("Date:       %s\n", meta.Date)
}
```

Add `"github.com/jason/incipit/internal/epub"` to the imports.

- [ ] **Step 2: Verify it works**

```bash
# Build a test epub is complex; verify it compiles and handles a missing file
go run main.go parse /nonexistent.epub
# Should print an error about opening the epub zip
```

- [ ] **Step 3: Run quality gates**

Run: `go vet ./... && gofmt -l . && go test ./...`
Expected: all clean

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add parse CLI command"
```

Questions before moving on?

---

## Task 9: Lookup — Open Library Client

**Files:**
- Create: `internal/lookup/lookup.go`
- Create: `internal/lookup/openlibrary.go`
- Create: `internal/lookup/testdata/ol_isbn.json`
- Test: `internal/lookup/openlibrary_test.go`

**Interfaces:**
- Consumes: `models.LookupResult`
- Produces: `lookup.ParseOLResponse([]byte) (*models.LookupResult, error)`, `lookup.OLClient` with `LookupByISBN(ctx, isbn)` and `LookupByTitle(ctx, title, author)`

> **Go note:** `net/http` is Go's HTTP client/server — all stdlib, no third-party needed. `http.Client` is like Python's `requests` but lower-level. You create a `http.Request`, send it with `client.Do(req)`, and read `resp.Body`. Always close the body — `defer resp.Body.Close()` is mandatory or you leak connections.

> **Go note:** `context.Context` is Go's cancellation and timeout mechanism. It's passed as the first argument to most functions that do I/O: `func Lookup(ctx context.Context, isbn string)`. The context carries deadlines and cancellation signals. If the caller cancels (e.g., the user hits Ctrl-C), the in-flight HTTP request is aborted. This is Go's answer to C#' `CancellationToken` but more pervasive — it's a convention, not a type you can ignore.

- [ ] **Step 1: Create a fixture JSON file**

Create `internal/lookup/testdata/ol_isbn.json`:

```json
{
  "ISBN:9780316129084": {
    "title": "Leviathan Wakes",
    "authors": [{"name": "James S. A. Corey"}],
    "publishers": [{"name": "Orbit Books"}],
    "publish_date": "2011",
    "number_of_pages": 577,
    "subjects": [
      {"name": "Space warfare"},
      {"name": "Fiction"},
      {"name": "series:The Expanse"}
    ],
    "cover": {
      "large": "https://covers.openlibrary.org/b/id/11295081-L.jpg"
    }
  }
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/lookup/openlibrary_test.go`:

```go
package lookup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseOLResponse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "ol_isbn.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	result, err := ParseOLResponse(data)
	if err != nil {
		t.Fatalf("ParseOLResponse failed: %v", err)
	}

	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
	if result.Author != "James S. A. Corey" {
		t.Errorf("expected author 'James S. A. Corey', got %q", result.Author)
	}
	if result.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got %q", result.Series)
	}
	if result.Publisher != "Orbit Books" {
		t.Errorf("expected publisher 'Orbit Books', got %q", result.Publisher)
	}
	if result.Pages != 577 {
		t.Errorf("expected pages 577, got %d", result.Pages)
	}
	if result.CoverURL != "https://covers.openlibrary.org/b/id/11295081-L.jpg" {
		t.Errorf("expected cover URL, got %q", result.CoverURL)
	}
	if len(result.Subjects) != 2 {
		t.Errorf("expected 2 subjects (excluding series), got %d", len(result.Subjects))
	}
}

func TestOLLookupByISBN(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("testdata", "ol_isbn.json"))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer ts.Close()

	client := NewOLClient(ts.URL)
	result, err := client.LookupByISBN(context.Background(), "9780316129084")
	if err != nil {
		t.Fatalf("LookupByISBN failed: %v", err)
	}
	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
}

func TestParseOLResponseEmpty(t *testing.T) {
	result, err := ParseOLResponse([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseOLResponse on empty object failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty response, got %+v", result)
	}
}

func TestParseOLResponseUserAgent(t *testing.T) {
	var capturedUA string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		data, _ := json.Marshal(map[string]interface{}{})
		w.Write(data)
	}))
	defer ts.Close()

	client := NewOLClient(ts.URL)
	client.LookupByISBN(context.Background(), "9780316129084")

	if capturedUA == "" {
		t.Error("expected User-Agent header to be set")
	}
}
```

> **Go note:** `httptest.NewServer` starts a real HTTP server on a random local port. It's like Python's `responses` library but actually serves HTTP — no mocking, real HTTP roundtrips. The server returns fixture data, so no external network is needed. This is the Go way to test HTTP clients.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/lookup/ -v`
Expected: FAIL with `undefined: ParseOLResponse`

- [ ] **Step 4: Write the lookup types and OL client**

Create `internal/lookup/lookup.go`:

```go
package lookup

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

type Client interface {
	LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error)
	LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error)
}
```

Create `internal/lookup/openlibrary.go`:

```go
package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/jason/incipit/internal/models"
)

type OLClient struct {
	baseURL string
	http    *http.Client
}

func NewOLClient(baseURL string) *OLClient {
	return &OLClient{
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (c *OLClient) LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/api/books?bibkeys=ISBN:%s&format=json&jscmd=data", c.baseURL, url.QueryEscape(isbn))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseOLResponse(body)
}

func (c *OLClient) LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/search.json?title=%s&author=%s&limit=1",
		c.baseURL, url.QueryEscape(title), url.QueryEscape(author))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseOLSearchResponse(body)
}

func (c *OLClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "incipit/0.1 (https://github.com/superversivesf/incipit)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return body, nil
}

// OL API response structures
type olISBNResponse map[string]struct {
	Title          string `json:"title"`
	Authors        []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate    string `json:"publish_date"`
	NumberOfPages  int    `json:"number_of_pages"`
	Subjects       []struct {
		Name string `json:"name"`
	} `json:"subjects"`
	Cover struct {
		Large string `json:"large"`
	} `json:"cover"`
}

type olSearchResponse struct {
	NumFound int `json:"numFound"`
	Docs     []struct {
		Title              string   `json:"title"`
		AuthorName         []string `json:"author_name"`
		FirstPublishYear   int      `json:"first_publish_year"`
		ISBN              []string `json:"isbn"`
		Subject           []string `json:"subject"`
		CoverI            int      `json:"cover_i"`
		NumberOfPagesMedian int     `json:"number_of_pages_median"`
	} `json:"docs"`
}

func ParseOLResponse(data []byte) (*models.LookupResult, error) {
	var resp olISBNResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Open Library response: %w", err)
	}

	for _, book := range resp {
		return olBookToResult(book), nil
	}
	return nil, nil
}

func ParseOLSearchResponse(data []byte) (*models.LookupResult, error) {
	var resp olSearchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Open Library search response: %w", err)
	}

	if len(resp.Docs) == 0 {
		return nil, nil
	}

	doc := resp.Docs[0]
	result := &models.LookupResult{
		Title:    doc.Title,
		Pages:    doc.NumberOfPagesMedian,
		Published: fmt.Sprintf("%d", doc.FirstPublishYear),
		Sources:  []string{"openlibrary"},
	}

	if len(doc.AuthorName) > 0 {
		result.Author = strings.Join(doc.AuthorName, ", ")
	}
	if len(doc.ISBN) > 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/isbn/%s-L.jpg", doc.ISBN[0])
	}
	if doc.CoverI > 0 {
		result.CoverURL = fmt.Sprintf("https://covers.openlibrary.org/b/id/%d-L.jpg", doc.CoverI)
	}

	for _, s := range doc.Subject {
		if strings.HasPrefix(s, "series:") {
			result.Series = strings.TrimPrefix(s, "series:")
		} else {
			result.Subjects = append(result.Subjects, s)
		}
	}

	return result, nil
}

func olBookToResult(book struct {
	Title          string `json:"title"`
	Authors        []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Publishers []struct {
		Name string `json:"name"`
	} `json:"publishers"`
	PublishDate    string `json:"publish_date"`
	NumberOfPages  int    `json:"number_of_pages"`
	Subjects       []struct {
		Name string `json:"name"`
	} `json:"subjects"`
	Cover struct {
		Large string `json:"large"`
	} `json:"cover"`
}) *models.LookupResult {
	result := &models.LookupResult{
		Title:     book.Title,
		Pages:     book.NumberOfPages,
		Published: book.PublishDate,
		Sources:   []string{"openlibrary"},
	}

	if len(book.Authors) > 0 {
		result.Author = book.Authors[0].Name
	}
	if len(book.Publishers) > 0 {
		result.Publisher = book.Publishers[0].Name
	}
	if book.Cover.Large != "" {
		result.CoverURL = book.Cover.Large
	}

	for _, s := range book.Subjects {
		if strings.HasPrefix(s.Name, "series:") {
			result.Series = strings.TrimPrefix(s.Name, "series:")
		} else {
			result.Subjects = append(result.Subjects, s.Name)
		}
	}

	return result
}
```

> **Go note:** Go interfaces are implicit — a type satisfies an interface by having the right methods, no `implements` keyword needed. `OLClient` satisfies `lookup.Client` because it has `LookupByISBN` and `LookupByTitle` methods, even though we never wrote `implements Client` anywhere. This is "duck typing" at compile time — like TypeScript's structural typing but enforced by the compiler.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/lookup/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/lookup/
git commit -m "feat: add Open Library lookup client"
```

Questions before moving on?

---

## Task 10: Lookup — Google Books Client

**Files:**
- Create: `internal/lookup/googlebooks.go`
- Create: `internal/lookup/testdata/gb_isbn.json`
- Test: `internal/lookup/googlebooks_test.go`

**Interfaces:**
- Produces: `lookup.GBClient` with `LookupByISBN` and `LookupByTitle`, `ParseGBResponse([]byte) (*models.LookupResult, error)`

- [ ] **Step 1: Create a fixture JSON file**

Create `internal/lookup/testdata/gb_isbn.json`:

```json
{
  "items": [{
    "volumeInfo": {
      "title": "Leviathan Wakes",
      "authors": ["James S. A. Corey"],
      "publishedDate": "2011-06-15",
      "description": "Two hundred years after migrating into space...",
      "categories": ["Fiction", "Science Fiction"],
      "averageRating": 4.5,
      "imageLinks": {
        "thumbnail": "http://books.google.com/books/content?id=abc123"
      }
    }
  }]
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/lookup/googlebooks_test.go`:

```go
package lookup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseGBResponse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "gb_isbn.json"))
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	result, err := ParseGBResponse(data)
	if err != nil {
		t.Fatalf("ParseGBResponse failed: %v", err)
	}

	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
	if result.Author != "James S. A. Corey" {
		t.Errorf("expected author 'James S. A. Corey', got %q", result.Author)
	}
	if result.Rating != 4.5 {
		t.Errorf("expected rating 4.5, got %f", result.Rating)
	}
	if result.Description != "Two hundred years after migrating into space..." {
		t.Errorf("expected description, got %q", result.Description)
	}
	if result.Published != "2011-06-15" {
		t.Errorf("expected published '2011-06-15', got %q", result.Published)
	}
	if len(result.Subjects) != 2 {
		t.Errorf("expected 2 categories, got %d", len(result.Subjects))
	}
}

func TestParseGBResponseEmpty(t *testing.T) {
	result, err := ParseGBResponse([]byte(`{"items": []}`))
	if err != nil {
		t.Fatalf("ParseGBResponse on empty items failed: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty items, got %+v", result)
	}
}

func TestGBLookupByISBN(t *testing.T) {
	data, _ := os.ReadFile(filepath.Join("testdata", "gb_isbn.json"))
	ts := serveFixture(data)
	defer ts.Close()

	client := NewGBClient(ts.URL)
	result, err := client.LookupByISBN(context.Background(), "9780316129084")
	if err != nil {
		t.Fatalf("LookupByISBN failed: %v", err)
	}
	if result.Title != "Leviathan Wakes" {
		t.Errorf("expected title 'Leviathan Wakes', got %q", result.Title)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/lookup/ -run TestParseGBResponse -v`
Expected: FAIL with `undefined: ParseGBResponse`

- [ ] **Step 4: Write the Google Books client**

Create `internal/lookup/googlebooks.go`:

```go
package lookup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jason/incipit/internal/models"
)

type GBClient struct {
	baseURL string
	http    *http.Client
}

func NewGBClient(baseURL string) *GBClient {
	return &GBClient{
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (c *GBClient) LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/books/v1/volumes?q=isbn:%s", c.baseURL, url.QueryEscape(isbn))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseGBResponse(body)
}

func (c *GBClient) LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error) {
	u := fmt.Sprintf("%s/books/v1/volumes?q=intitle:%s+inauthor:%s",
		c.baseURL, url.QueryEscape(title), url.QueryEscape(author))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return ParseGBResponse(body)
}

func (c *GBClient) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "incipit/0.1 (https://github.com/superversivesf/incipit)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	return body, nil
}

type gbResponse struct {
	Items []struct {
		VolumeInfo struct {
			Title         string   `json:"title"`
			Authors       []string `json:"authors"`
			PublishedDate string   `json:"publishedDate"`
			Description   string   `json:"description"`
			Categories    []string `json:"categories"`
			AverageRating float64  `json:"averageRating"`
			ImageLinks    struct {
				Thumbnail string `json:"thumbnail"`
			} `json:"imageLinks"`
		} `json:"volumeInfo"`
	} `json:"items"`
}

func ParseGBResponse(data []byte) (*models.LookupResult, error) {
	var resp gbResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing Google Books response: %w", err)
	}

	if len(resp.Items) == 0 {
		return nil, nil
	}

	vi := resp.Items[0].VolumeInfo
	result := &models.LookupResult{
		Title:       vi.Title,
		Published:   vi.PublishedDate,
		Description: vi.Description,
		Rating:      vi.AverageRating,
		Subjects:    vi.Categories,
		Sources:     []string{"googlebooks"},
	}

	if len(vi.Authors) > 0 {
		result.Author = vi.Authors[0]
	}
	if vi.ImageLinks.Thumbnail != "" {
		result.CoverURL = vi.ImageLinks.Thumbnail
	}

	return result, nil
}
```

- [ ] **Step 5: Add the shared test helper to openlibrary_test.go or a new test helper file**

Create `internal/lookup/testhelpers_test.go`:

```go
package lookup

import (
	"net/http"
	"net/http/httptest"
)

func serveFixture(data []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/lookup/ -v`
Expected: all tests PASS

- [ ] **Step 7: Commit**

```bash
git add internal/lookup/
git commit -m "feat: add Google Books lookup client"
```

Questions before moving on?

---

## Task 11: Lookup — Merge Function

**Files:**
- Create: `internal/lookup/merge.go`
- Test: `internal/lookup/merge_test.go`

**Interfaces:**
- Produces: `lookup.Merge(ol, gb *models.LookupResult) *models.LookupResult`

> **Design note:** Merge precedence: OL wins series/subjects/cover; GB wins rating/description/published date; first non-empty wins for title/author/pages/publisher.

- [ ] **Step 1: Write the failing test**

Create `internal/lookup/merge_test.go`:

```go
package lookup

import (
	"testing"

	"github.com/jason/incipit/internal/models"
)

func TestMerge(t *testing.T) {
	ol := &models.LookupResult{
		Title:    "Leviathan Wakes",
		Author:   "James S. A. Corey",
		Series:   "The Expanse",
		Subjects: []string{"Space warfare", "Fiction"},
		CoverURL: "https://covers.openlibrary.org/b/id/11295081-L.jpg",
		Pages:    577,
		Publisher: "Orbit Books",
		Sources:  []string{"openlibrary"},
	}

	gb := &models.LookupResult{
		Title:       "", // empty, OL wins
		Author:      "", // empty, OL wins
		Published:   "2011-06-15",
		Rating:      4.5,
		Description: "Two hundred years after migrating into space...",
		Sources:     []string{"googlebooks"},
	}

	merged := Merge(ol, gb)

	if merged.Title != "Leviathan Wakes" {
		t.Errorf("title: expected 'Leviathan Wakes', got %q", merged.Title)
	}
	if merged.Series != "The Expanse" {
		t.Errorf("series: expected 'The Expanse', got %q", merged.Series)
	}
	if merged.Rating != 4.5 {
		t.Errorf("rating: expected 4.5, got %f", merged.Rating)
	}
	if merged.Description != "Two hundred years after migrating into space..." {
		t.Errorf("description: expected from GB, got %q", merged.Description)
	}
	if merged.Published != "2011-06-15" {
		t.Errorf("published: expected '2011-06-15', got %q", merged.Published)
	}
	if merged.CoverURL != "https://covers.openlibrary.org/b/id/11295081-L.jpg" {
		t.Errorf("cover: expected from OL, got %q", merged.CoverURL)
	}
	if len(merged.Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(merged.Sources))
	}
}

func TestMergeNils(t *testing.T) {
	ol := &models.LookupResult{Title: "From OL"}
	gb := &models.LookupResult{Title: "", Rating: 4.0}

	merged := Merge(ol, gb)
	if merged.Title != "From OL" {
		t.Errorf("expected 'From OL', got %q", merged.Title)
	}
	if merged.Rating != 4.0 {
		t.Errorf("expected rating 4.0, got %f", merged.Rating)
	}

	merged = Merge(nil, gb)
	if merged.Title != "" {
		t.Errorf("expected empty title, got %q", merged.Title)
	}
	if merged.Rating != 4.0 {
		t.Errorf("expected rating 4.0 from GB, got %f", merged.Rating)
	}

	merged = Merge(ol, nil)
	if merged.Title != "From OL" {
		t.Errorf("expected 'From OL', got %q", merged.Title)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/lookup/ -run TestMerge -v`
Expected: FAIL with `undefined: Merge`

- [ ] **Step 3: Write the merge function**

Create `internal/lookup/merge.go`:

```go
package lookup

import "github.com/jason/incipit/internal/models"

func Merge(ol, gb *models.LookupResult) *models.LookupResult {
	if ol == nil && gb == nil {
		return nil
	}
	if ol == nil {
		return gb
	}
	if gb == nil {
		return ol
	}

	merged := &models.LookupResult{}

	// OL wins: series, subjects, cover
	merged.Series = ol.Series
	merged.Subjects = ol.Subjects
	merged.CoverURL = ol.CoverURL

	// GB wins: rating, description, published date
	merged.Rating = gb.Rating
	merged.Description = gb.Description
	merged.Published = gb.Published

	// First non-empty wins: title, author, pages, publisher
	merged.Title = firstNonEmpty(ol.Title, gb.Title)
	merged.Author = firstNonEmpty(ol.Author, gb.Author)
	merged.Pages = firstNonZero(ol.Pages, gb.Pages)
	merged.Publisher = firstNonEmpty(ol.Publisher, gb.Publisher)

	// Combine sources
	merged.Sources = append(merged.Sources, ol.Sources...)
	merged.Sources = append(merged.Sources, gb.Sources...)

	return merged
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
```

> **Go note:** Variadic parameters (`vals ...string`) let you pass any number of arguments — like `params object[]` in C# or `*args` in Python. The function sees them as a slice. You call it as `firstNonEmpty(a, b, c)` or `firstNonEmpty(slice...)` with the `...` spread operator.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/lookup/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/lookup/merge.go internal/lookup/merge_test.go
git commit -m "feat: add lookup merge function with OL/GB precedence"
```

Questions before moving on?

---

## Task 12: main.go — lookup Command

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `lookup.NewOLClient`, `lookup.NewGBClient`, `lookup.Merge`

- [ ] **Step 1: Update the `lookup` case and add the function**

In `main.go`, replace the `lookup` case:

```go
	case "lookup":
		runLookup(os.Args[2:])
```

Add the function:

```go
func runLookup(args []string) {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	isbn := fs.String("isbn", "", "ISBN to look up")
	title := fs.String("title", "", "book title")
	author := fs.String("author", "", "book author")
	fs.Parse(args)

	if *isbn == "" && *title == "" {
		fmt.Fprintln(os.Stderr, "usage: incipit lookup [--isbn X | --title T --author A]")
		os.Exit(2)
	}

	ctx := context.Background()

	var olResult, gbResult *models.LookupResult
	var err error

	ol := lookup.NewOLClient("https://openlibrary.org")
	gb := lookup.NewGBClient("https://www.googleapis.com")

	if *isbn != "" {
		olResult, err = ol.LookupByISBN(ctx, *isbn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Open Library lookup failed: %v\n", err)
		}
		gbResult, err = gb.LookupByISBN(ctx, *isbn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Google Books lookup failed: %v\n", err)
		}
	} else {
		olResult, err = ol.LookupByTitle(ctx, *title, *author)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Open Library lookup failed: %v\n", err)
		}
		gbResult, err = gb.LookupByTitle(ctx, *title, *author)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Google Books lookup failed: %v\n", err)
		}
	}

	merged := lookup.Merge(olResult, gbResult)
	if merged == nil {
		fmt.Println("No results found")
		os.Exit(0)
	}

	fmt.Printf("Title:       %s\n", merged.Title)
	fmt.Printf("Author:      %s\n", merged.Author)
	if merged.Series != "" {
		fmt.Printf("Series:      %s\n", merged.Series)
	}
	if merged.Rating > 0 {
		fmt.Printf("Rating:      %.1f/5\n", merged.Rating)
	}
	if merged.Pages > 0 {
		fmt.Printf("Pages:       %d\n", merged.Pages)
	}
	if merged.Publisher != "" {
		fmt.Printf("Publisher:   %s\n", merged.Publisher)
	}
	if merged.Published != "" {
		fmt.Printf("Published:   %s\n", merged.Published)
	}
	if len(merged.Subjects) > 0 {
		fmt.Printf("Subjects:    %v\n", merged.Subjects)
	}
	if merged.Description != "" {
		fmt.Printf("Description: %s\n", merged.Description)
	}
	if merged.CoverURL != "" {
		fmt.Printf("Cover URL:   %s\n", merged.CoverURL)
	}
}
```

Add imports:
```go
	"context"
	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
```

Remove the `var _ = strconv.Itoa` line (strconv no longer needed) and add `var _ = context.TODO`.

- [ ] **Step 2: Verify it compiles**

Run: `go build -o incipit .`
Expected: builds successfully

- [ ] **Step 3: Run quality gates**

Run: `go vet ./... && gofmt -l . && go test ./...`
Expected: all clean

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add lookup CLI command with OL+GB merge"
```

Questions before moving on?

---

## Task 13: Storage — File Saving and Hashing

**Files:**
- Create: `internal/storage/storage.go`
- Test: `internal/storage/storage_test.go`

**Interfaces:**
- Produces: `storage.New(dir string) *Storage`, `Storage.SaveBookFile(bookID int64, sourcePath string) error`, `Storage.SaveCover(bookID int64, imageData []byte) error`, `Storage.HashFile(path string) (string, error)`, `Storage.BookFilePath(bookID int64) string`, `Storage.CoverPath(bookID int64) string`

- [ ] **Step 1: Write the failing test**

Create `internal/storage/storage_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveBookFile(t *testing.T) {
	s := New(t.TempDir())

	// Create a fake "epub" file
	src := filepath.Join(t.TempDir(), "source.epub")
	os.WriteFile(src, []byte("epub content"), 0644)

	err := s.SaveBookFile(1, src)
	if err != nil {
		t.Fatalf("SaveBookFile failed: %v", err)
	}

	expected := filepath.Join(s.rootDir, "files", "1.epub")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected file at %s, not found", expected)
	}
}

func TestSaveCover(t *testing.T) {
	s := New(t.TempDir())

	err := s.SaveCover(1, []byte("jpeg data"))
	if err != nil {
		t.Fatalf("SaveCover failed: %v", err)
	}

	expected := filepath.Join(s.rootDir, "covers", "1.jpg")
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Errorf("expected cover at %s, not found", expected)
	}
}

func TestHashFile(t *testing.T) {
	// MD5 of "hello" is 5d41402abc4b2a76b9719d911017c592
	path := filepath.Join(t.TempDir(), "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	s := New(t.TempDir())
	hash, err := s.HashFile(path)
	if err != nil {
		t.Fatalf("HashFile failed: %v", err)
	}

	expected := "5d41402abc4b2a76b9719d911017c592"
	if hash != expected {
		t.Errorf("expected hash %s, got %s", expected, hash)
	}
}

func TestBookFilePath(t *testing.T) {
	s := New("/data")
	expected := "/data/files/42.epub"
	got := s.BookFilePath(42)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestCoverPath(t *testing.T) {
	s := New("/data")
	expected := "/data/covers/42.jpg"
	got := s.CoverPath(42)
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestLazyDirCreation(t *testing.T) {
	s := New(t.TempDir())
	// Directories don't exist yet — SaveBookFile should create them
	src := filepath.Join(t.TempDir(), "src.epub")
	os.WriteFile(src, []byte("data"), 0644)

	err := s.SaveBookFile(1, src)
	if err != nil {
		t.Fatalf("SaveBookFile should create dirs: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -v`
Expected: FAIL with `undefined: New`

- [ ] **Step 3: Write the storage implementation**

Create `internal/storage/storage.go`:

```go
package storage

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Storage struct {
	rootDir string
}

func New(rootDir string) *Storage {
	return &Storage{rootDir: rootDir}
}

func (s *Storage) SaveBookFile(bookID int64, sourcePath string) error {
	dst := s.BookFilePath(bookID)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating files dir: %w", err)
	}
	return copyFile(sourcePath, dst)
}

func (s *Storage) SaveCover(bookID int64, imageData []byte) error {
	dst := s.CoverPath(bookID)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating covers dir: %w", err)
	}
	return os.WriteFile(dst, imageData, 0644)
}

func (s *Storage) HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening file for hashing: %w", err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Storage) BookFilePath(bookID int64) string {
	return filepath.Join(s.rootDir, "files", fmt.Sprintf("%d.epub", bookID))
}

func (s *Storage) CoverPath(bookID int64) string {
	return filepath.Join(s.rootDir, "covers", fmt.Sprintf("%d.jpg", bookID))
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("creating destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}
	return nil
}
```

> **Go note:** `io.Copy(dst, src)` is the idiomatic way to copy streams in Go — it's like `shutil.copyfile` in Python but works with any `io.Reader`/`io.Writer`. It handles buffering internally. The `defer` pattern ensures both files are closed even if the copy fails.

> **Go note:** `md5.New()` returns a `hash.Hash` which implements `io.Writer`. You `io.Copy` the file into it, then call `h.Sum(nil)` to get the digest. The `nil` argument means "give me a new slice" — you can also pass an existing byte slice to append to.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/storage/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/storage/
git commit -m "feat: add storage package with file saving and MD5 hashing"
```

Questions before moving on?

---

## Task 14: Search — Searcher Interface and LikeSearcher

**Files:**
- Create: `internal/search/search.go`
- Create: `internal/search/like.go`
- Test: `internal/search/like_test.go`

**Interfaces:**
- Consumes: `db.DB` (queries), `models.Book`
- Produces: `search.Searcher` interface, `search.LikeSearcher`, `search.Opts`

> **Go note:** This is where Go interfaces shine. We define a `Searcher` interface and a `LikeSearcher` implementation. Later, `FTS5Searcher` can replace `LikeSearcher` without changing any callers — just swap the implementation in `server.New()`. This is the "strategy pattern" but Go makes it implicit — no class hierarchy needed.

- [ ] **Step 1: Write the failing test**

Create `internal/search/like_test.go`:

```go
package search

import (
	"testing"

	"github.com/jason/incipit/internal/db"
)

func TestLikeSearcher(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()
	d.Migrate()

	// Seed a few books
	d.InsertBook(&models.Book{Title: "Leviathan Wakes", Author: "Corey", FilePath: "files/1.epub"})
	d.InsertBook(&models.Book{Title: "Caliban's War", Author: "Corey", FilePath: "files/2.epub"})
	d.InsertBook(&models.Book{Title: "Dune", Author: "Herbert", FilePath: "files/3.epub"})

	s := NewLikeSearcher(d)
	books, total, err := s.Search("levi", Opts{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 result, got %d", total)
	}
	if len(books) != 1 {
		t.Fatalf("expected 1 book, got %d", len(books))
	}
	if books[0].Title != "Leviathan Wakes" {
		t.Errorf("expected 'Leviathan Wakes', got %q", books[0].Title)
	}
}

func TestLikeSearcherEmpty(t *testing.T) {
	d, _ := db.Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	s := NewLikeSearcher(d)
	books, total, err := s.Search("", Opts{})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 results for empty query, got %d", total)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/ -v`
Expected: FAIL with `undefined: NewLikeSearcher`

- [ ] **Step 3: Add InsertBook to db package (needed by search test)**

Add to `internal/db/books.go` (create it):

```go
package db

import (
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) InsertBook(b *models.Book) (int64, error) {
	if b.TitleSort == "" {
		b.TitleSort = models.SortTitle(b.Title)
	}
	if b.AuthorSort == "" {
		b.AuthorSort = models.SortTitle(b.Author)
	}

	result, err := d.db.Exec(
		`INSERT INTO books (title, title_sort, author, author_sort, series, series_index,
		   isbn, description, publisher, published, pages, rating, cover_path,
		   file_path, file_hash, file_size)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.Title, b.TitleSort, b.Author, b.AuthorSort, b.Series, b.SeriesIndex,
		b.ISBN, b.Description, b.Publisher, b.Published, b.Pages, b.Rating,
		b.CoverPath, b.FilePath, b.FileHash, b.FileSize,
	)
	if err != nil {
		return 0, fmt.Errorf("inserting book: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting book ID: %w", err)
	}
	b.ID = id
	return id, nil
}

func (d *DB) GetBook(id int64) (*models.Book, error) {
	var b models.Book
	err := d.db.QueryRow(
		`SELECT id, title, title_sort, author, author_sort, series, series_index,
		   isbn, description, publisher, published, pages, rating, cover_path,
		   file_path, file_hash, file_size, added, updated
		 FROM books WHERE id = ?`, id,
	).Scan(&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort, &b.Series,
		&b.SeriesIndex, &b.ISBN, &b.Description, &b.Publisher, &b.Published,
		&b.Pages, &b.Rating, &b.CoverPath, &b.FilePath, &b.FileHash, &b.FileSize,
		&b.Added, &b.Updated)
	if err != nil {
		return nil, fmt.Errorf("getting book %d: %w", id, err)
	}
	return &b, nil
}

func (d *DB) ListBooks(limit, offset int) ([]models.Book, int, error) {
	var total int
	err := d.db.QueryRow("SELECT COUNT(*) FROM books").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting books: %w", err)
	}

	rows, err := d.db.Query(
		`SELECT id, title, title_sort, author, author_sort, series, series_index,
		   isbn, description, publisher, published, pages, rating, cover_path,
		   file_path, file_hash, file_size, added, updated
		 FROM books ORDER BY title_sort LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing books: %w", err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort,
			&b.Series, &b.SeriesIndex, &b.ISBN, &b.Description, &b.Publisher,
			&b.Published, &b.Pages, &b.Rating, &b.CoverPath, &b.FilePath,
			&b.FileHash, &b.FileSize, &b.Added, &b.Updated); err != nil {
			return nil, 0, fmt.Errorf("scanning book row: %w", err)
		}
		books = append(books, b)
	}
	return books, total, rows.Err()
}

func (d *DB) GetBookByFileHash(hash string) (*models.Book, error) {
	var b models.Book
	err := d.db.QueryRow(
		`SELECT id, title, author, file_hash FROM books WHERE file_hash = ?`, hash,
	).Scan(&b.ID, &b.Title, &b.Author, &b.FileHash)
	if err != nil {
		return nil, fmt.Errorf("book with hash %s: %w", hash, err)
	}
	return &b, nil
}

func (d *DB) DeleteBook(id int64) error {
	_, err := d.db.Exec("DELETE FROM books WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting book %d: %w", id, err)
	}
	return nil
}

func (d *DB) UpdateBook(b *models.Book) error {
	if b.TitleSort == "" {
		b.TitleSort = models.SortTitle(b.Title)
	}
	if b.AuthorSort == "" {
		b.AuthorSort = models.SortTitle(b.Author)
	}

	_, err := d.db.Exec(
		`UPDATE books SET title=?, title_sort=?, author=?, author_sort=?, series=?,
		   series_index=?, isbn=?, description=?, publisher=?, published=?,
		   pages=?, rating=?, cover_path=?, updated=datetime('now')
		 WHERE id = ?`,
		b.Title, b.TitleSort, b.Author, b.AuthorSort, b.Series, b.SeriesIndex,
		b.ISBN, b.Description, b.Publisher, b.Published, b.Pages, b.Rating,
		b.CoverPath, b.ID,
	)
	if err != nil {
		return fmt.Errorf("updating book %d: %w", b.ID, err)
	}
	return nil
}
```

- [ ] **Step 4: Write the search interface and LikeSearcher**

Create `internal/search/search.go`:

```go
package search

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

type Opts struct {
	Limit  int
	Offset int
}

type Searcher interface {
	Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error)
}
```

Create `internal/search/like.go`:

```go
package search

import (
	"context"
	"fmt"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)

type LikeSearcher struct {
	db *db.DB
}

func NewLikeSearcher(db *db.DB) *LikeSearcher {
	return &LikeSearcher{db: db}
}

func (s *LikeSearcher) Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error) {
	if q == "" {
		return nil, 0, nil
	}

	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}

	pattern := "%" + q + "%"

	var total int
	err := s.db.DB().QueryRow(
		"SELECT COUNT(*) FROM books WHERE title LIKE ? OR author LIKE ?",
		pattern, pattern,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting search results: %w", err)
	}

	rows, err := s.db.DB().Query(
		`SELECT id, title, title_sort, author, author_sort, series, series_index,
		   isbn, description, publisher, published, pages, rating, cover_path,
		   file_path, file_hash, file_size, added, updated
		 FROM books WHERE title LIKE ? OR author LIKE ?
		 ORDER BY title_sort LIMIT ? OFFSET ?`,
		pattern, pattern, limit, opts.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("searching books: %w", err)
	}
	defer rows.Close()

	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort,
			&b.Series, &b.SeriesIndex, &b.ISBN, &b.Description, &b.Publisher,
			&b.Published, &b.Pages, &b.Rating, &b.CoverPath, &b.FilePath,
			&b.FileHash, &b.FileSize, &b.Added, &b.Updated); err != nil {
			return nil, 0, fmt.Errorf("scanning book row: %w", err)
		}
		books = append(books, b)
	}
	return books, total, rows.Err()
}
```

Add a `DB()` accessor to `internal/db/db.go`:

```go
func (d *DB) DB() *sql.DB {
	return d.db
}
```

Add the `models` import to the search test:

```go
import (
	"testing"

	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/models"
)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/search/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/search/ internal/db/books.go internal/db/db.go
git commit -m "feat: add search package with Searcher interface and LikeSearcher, plus book CRUD"
```

Questions before moving on?

---

## Task 15: Models — MergeMetadata Function

**Files:**
- Create: `internal/models/merge.go`
- Test: `internal/models/merge_test.go`

**Interfaces:**
- Consumes: `models.Metadata`, `models.LookupResult`
- Produces: `models.MergeMetadata(epub *Metadata, lookup *LookupResult) Book`

> **Design note:** This is the second merge — combining EPUB metadata with lookup results into a final `Book` record. ISBN is the source of truth. Lookup wins non-empty fields, EPUB fills gaps, subjects/tags merge as union. This is a pure function — easy to tune later once we have real data.

- [ ] **Step 1: Write the failing test**

Create `internal/models/merge_test.go`:

```go
package models

import "testing"

func TestMergeMetadata(t *testing.T) {
	epub := &Metadata{
		Title:      "Leviathan Wakes",
		Creator:    "James S. A. Corey",
		Identifier: "9780316129084",
		Language:   "en",
		Publisher:  "Orbit Books",
		Date:       "2011-06-15",
	}

	lookup := &LookupResult{
		Title:     "Leviathan Wakes (The Expanse #1)",
		Author:    "James S. A. Corey",
		Series:    "The Expanse",
		Subjects:  []string{"Space warfare", "Fiction"},
		CoverURL:  "https://covers.openlibrary.org/b/id/11295081-L.jpg",
		Pages:     577,
		Publisher: "Orbit Books",
		Published: "2011-06-15",
		Rating:    4.5,
	}

	book := MergeMetadata(epub, lookup)

	if book.ISBN != "9780316129084" {
		t.Errorf("expected ISBN from EPUB, got %q", book.ISBN)
	}
	// Lookup wins for title (non-empty)
	if book.Title != "Leviathan Wakes (The Expanse #1)" {
		t.Errorf("expected title from lookup, got %q", book.Title)
	}
	if book.Series != "The Expanse" {
		t.Errorf("expected series 'The Expanse', got %q", book.Series)
	}
	if book.Pages != 577 {
		t.Errorf("expected pages 577, got %d", book.Pages)
	}
	if book.Rating != 4.5 {
		t.Errorf("expected rating 4.5, got %f", book.Rating)
	}
	if book.Publisher != "Orbit Books" {
		t.Errorf("expected publisher 'Orbit Books', got %q", book.Publisher)
	}
}

func TestMergeMetadataLookupNil(t *testing.T) {
	epub := &Metadata{
		Title:      "Test Book",
		Creator:    "Test Author",
		Identifier:  "1234567890",
	}

	book := MergeMetadata(epub, nil)

	if book.Title != "Test Book" {
		t.Errorf("expected title from EPUB, got %q", book.Title)
	}
	if book.Author != "Test Author" {
		t.Errorf("expected author from EPUB, got %q", book.Author)
	}
	if book.ISBN != "1234567890" {
		t.Errorf("expected ISBN from EPUB, got %q", book.ISBN)
	}
}

func TestMergeMetadataEpubNil(t *testing.T) {
	lookup := &LookupResult{
		Title:  "From Lookup",
		Author: "Lookup Author",
	}

	book := MergeMetadata(nil, lookup)

	if book.Title != "From Lookup" {
		t.Errorf("expected title from lookup, got %q", book.Title)
	}
	if book.Author != "Lookup Author" {
		t.Errorf("expected author from lookup, got %q", book.Author)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestMergeMetadata -v`
Expected: FAIL with `undefined: MergeMetadata`

- [ ] **Step 3: Write MergeMetadata**

Append to `internal/models/merge.go`:

```go
func MergeMetadata(epub *Metadata, lookup *LookupResult) Book {
	book := Book{}

	if epub != nil {
		book.Title = epub.Title
		book.Author = epub.Creator
		book.ISBN = epub.Identifier
		book.Publisher = epub.Publisher
		book.Published = epub.Date
	}

	if lookup != nil {
		// Lookup wins non-empty fields, EPUB fills gaps
		if lookup.Title != "" {
			book.Title = lookup.Title
		}
		if lookup.Author != "" {
			book.Author = lookup.Author
		}
		if lookup.Publisher != "" {
			book.Publisher = lookup.Publisher
		}
		if lookup.Published != "" {
			book.Published = lookup.Published
		}
		// Lookup-only fields
		book.Series = lookup.Series
		book.Pages = lookup.Pages
		book.Rating = lookup.Rating
		book.Description = lookup.Description
	}

	// Compute sort fields
	book.TitleSort = SortTitle(book.Title)
	book.AuthorSort = SortTitle(book.Author)

	return book
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/merge.go internal/models/merge_test.go
git commit -m "feat: add MergeMetadata function for EPUB+lookup merge"
```

Questions before moving on?

---

## Task 16: DB — Metadata Cache

**Files:**
- Create: `internal/db/cache.go`
- Test: `internal/db/cache_test.go`

**Interfaces:**
- Produces: `db.DB.CacheGet(isbn, source string) (string, error)`, `db.DB.CachePut(isbn, title, author, source, response string) error`

> **Design note:** The `lookup` package doesn't import `db` — caching is handled by the caller. This keeps `lookup` testable without a DB. The `add` command wraps lookups with cache get/put.

- [ ] **Step 1: Write the failing test**

Create `internal/db/cache_test.go`:

```go
package db

import "testing"

func TestCachePutAndGet(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	err := d.CachePut("9780316129084", "Leviathan Wakes", "Corey", "openlibrary", `{"title":"LW"}`)
	if err != nil {
		t.Fatalf("CachePut failed: %v", err)
	}

	cached, err := d.CacheGet("9780316129084", "openlibrary")
	if err != nil {
		t.Fatalf("CacheGet failed: %v", err)
	}
	if cached != `{"title":"LW"}` {
		t.Errorf("expected cached response, got %q", cached)
	}
}

func TestCacheGetMiss(t *testing.T) {
	d, _ := Open(t.TempDir() + "/test.db")
	defer d.Close()
	d.Migrate()

	_, err := d.CacheGet("nonexistent", "openlibrary")
	if err == nil {
		t.Fatal("expected error on cache miss, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestCache -v`
Expected: FAIL with `undefined: d.CachePut`

- [ ] **Step 3: Write the cache implementation**

Create `internal/db/cache.go`:

```go
package db

import (
	"database/sql"
	"fmt"
)

func (d *DB) CacheGet(isbn, source string) (string, error) {
	var response string
	err := d.db.QueryRow(
		"SELECT response FROM metadata_cache WHERE isbn = ? AND source = ?",
		isbn, source,
	).Scan(&response)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("cache miss for isbn=%s source=%s: %w", isbn, source, err)
		}
		return "", fmt.Errorf("getting from cache: %w", err)
	}
	return response, nil
}

func (d *DB) CachePut(isbn, title, author, source, response string) error {
	_, err := d.db.Exec(
		`INSERT INTO metadata_cache (isbn, title, author, source, response)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(isbn, source) DO UPDATE SET response = excluded.response, cached_at = datetime('now')`,
		isbn, title, author, source, response,
	)
	if err != nil {
		return fmt.Errorf("putting to cache: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/cache.go internal/db/cache_test.go
git commit -m "feat: add metadata cache to db package"
```

Questions before moving on?

---

## Task 17: main.go — add Command (End-to-End)

**Files:**
- Modify: `main.go`

**Interfaces:**
- Consumes: `epub.Parse`, `lookup.Lookup`, `models.MergeMetadata`, `db.InsertBook`, `storage.SaveBookFile`, `storage.SaveCover`, `storage.HashFile`, `db.CacheGet`, `db.CachePut`

> This is the capstone of Phase 1. The `add` command ties everything together: parse EPUB → lookup metadata → merge → store in DB → copy file → download cover. This is where you see how all the packages compose through `main.go`.

- [ ] **Step 1: Update the `add` case and add the function**

In `main.go`, replace the `add` case:

```go
	case "add":
		runAdd(os.Args[2:])
```

Add the function and imports:

```go
import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/jason/incipit/internal/config"
	"github.com/jason/incipit/internal/db"
	"github.com/jason/incipit/internal/epub"
	"github.com/jason/incipit/internal/lookup"
	"github.com/jason/incipit/internal/models"
	"github.com/jason/incipit/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	noLookup := fs.Bool("no-lookup", false, "skip metadata lookup")
	dryRun := fs.Bool("dry-run", false, "preview without saving")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: incipit add <path> [--no-lookup] [--dry-run]")
		os.Exit(2)
	}
	path := fs.Arg(0)

	// Step 1: Parse EPUB
	fmt.Println("Parsing EPUB...")
	meta, err := epub.Parse(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing epub: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Title: %s\n", meta.Title)
	fmt.Printf("  Author: %s\n", meta.Creator)
	if meta.Identifier != "" {
		fmt.Printf("  ISBN: %s\n", meta.Identifier)
	}

	// Step 2: Lookup metadata (unless --no-lookup)
	var lookupResult *models.LookupResult
	if !*noLookup {
		fmt.Println("\nLooking up metadata...")
		ctx := context.Background()
		ol := lookup.NewOLClient("https://openlibrary.org")
		gb := lookup.NewGBClient("https://www.googleapis.com")

		if meta.Identifier != "" {
			olResult, olErr := ol.LookupByISBN(ctx, meta.Identifier)
			if olErr != nil {
				fmt.Fprintf(os.Stderr, "  Open Library: %v\n", olErr)
			}
			gbResult, gbErr := gb.LookupByISBN(ctx, meta.Identifier)
			if gbErr != nil {
				fmt.Fprintf(os.Stderr, "  Google Books: %v\n", gbErr)
			}
			lookupResult = lookup.Merge(olResult, gbResult)
		} else if meta.Title != "" {
			olResult, olErr := ol.LookupByTitle(ctx, meta.Title, meta.Creator)
			if olErr != nil {
				fmt.Fprintf(os.Stderr, "  Open Library: %v\n", olErr)
			}
			gbResult, gbErr := gb.LookupByTitle(ctx, meta.Title, meta.Creator)
			if gbErr != nil {
				fmt.Fprintf(os.Stderr, "  Google Books: %v\n", gbErr)
			}
			lookupResult = lookup.Merge(olResult, gbResult)
		}

		if lookupResult != nil {
			fmt.Printf("  Found: %s by %s\n", lookupResult.Title, lookupResult.Author)
			if lookupResult.Series != "" {
				fmt.Printf("  Series: %s\n", lookupResult.Series)
			}
			if lookupResult.Rating > 0 {
				fmt.Printf("  Rating: %.1f/5\n", lookupResult.Rating)
			}
		} else {
			fmt.Println("  No lookup results found")
		}
	}

	// Step 3: Merge metadata
	book := models.MergeMetadata(meta, lookupResult)

	// Step 4: Hash the file
	cfg := config.Load()
	s := storage.New(cfg.StorageDir)
	hash, err := s.HashFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error hashing file: %v\n", err)
		os.Exit(1)
	}
	book.FileHash = hash

	// Get file size
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error getting file info: %v\n", err)
		os.Exit(1)
	}
	book.FileSize = info.Size()

	if *dryRun {
		fmt.Println("\n[dry-run] Would add:")
		fmt.Printf("  Title: %s\n", book.Title)
		fmt.Printf("  Author: %s\n", book.Author)
		if book.Series != "" {
			fmt.Printf("  Series: %s\n", book.Series)
		}
		fmt.Printf("  ISBN: %s\n", book.ISBN)
		fmt.Printf("  File hash: %s\n", book.FileHash)
		fmt.Printf("  File size: %d bytes\n", book.FileSize)
		return
	}

	// Step 5: Open DB and save
	d, err := db.Open(cfg.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	defer d.Close()
	d.Migrate()

	// Set file path (relative to storage dir)
	// We'll use the book ID, so we insert first, then set the path
	book.FilePath = "" // will be set after insert

	bookID, err := d.InsertBook(&book)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error inserting book: %v\n", err)
		os.Exit(1)
	}
	book.ID = bookID
	book.FilePath = fmt.Sprintf("files/%d.epub", bookID)

	// Update the file path
	d.UpdateBook(&book)

	// Step 6: Copy EPUB file to storage
	if err := s.SaveBookFile(bookID, path); err != nil {
		fmt.Fprintf(os.Stderr, "error saving book file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n  Book ID: %d\n", bookID)
	fmt.Printf("  File: %s\n", s.BookFilePath(bookID))

	// Step 7: Download cover if available
	if lookupResult != nil && lookupResult.CoverURL != "" {
		resp, err := http.Get(lookupResult.CoverURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Cover download failed: %v\n", err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				coverData, err := io.ReadAll(resp.Body)
				if err == nil {
					s.SaveCover(bookID, coverData)
					book.CoverPath = fmt.Sprintf("covers/%d.jpg", bookID)
					d.UpdateBook(&book)
					fmt.Printf("  Cover: %s\n", s.CoverPath(bookID))
				}
			}
		}
	}

	// Step 8: Print summary
	fmt.Printf("\nAdded: %s by %s", book.Title, book.Author)
	if book.Series != "" {
		fmt.Printf(" (%s)", book.Series)
	}
	fmt.Println()
}
```

Remove the placeholder `var _ = ...` lines since all imports are now used.

- [ ] **Step 2: Verify it compiles**

Run: `go build -o incipit .`
Expected: builds successfully

- [ ] **Step 3: Run quality gates**

Run: `go vet ./... && gofmt -l . && go test ./...`
Expected: all clean, all tests pass

- [ ] **Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add end-to-end add command — parse, lookup, merge, store"
```

Questions before moving on?

---

## Phase 1 Completion Checklist

- [ ] `go run main.go init` creates a database with all tables
- [ ] `go run main.go add-user --username admin --password secret --role admin` creates a user
- [ ] `go run main.go list-users` shows the user
- [ ] `go run main.go parse <path.epub>` prints metadata
- [ ] `go run main.go lookup --isbn 9780316129084` looks up a book
- [ ] `go run main.go add <path.epub>` adds a book end-to-end
- [ ] `go run main.go add <path.epub> --no-lookup` adds with EPUB metadata only
- [ ] `go run main.go add <path.epub> --dry-run` previews without saving
- [ ] `go vet ./...` is clean
- [ ] `gofmt -l .` is empty
- [ ] `go test ./...` passes
- [ ] All code committed and pushed

**Phase 1 delivers a working CLI library tool. Phase 2 (web server + OPDS) and Phase 3 (sync + polish) plans are written separately.**