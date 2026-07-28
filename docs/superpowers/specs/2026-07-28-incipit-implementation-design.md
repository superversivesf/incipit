# Incipit — Implementation Design

**Status:** Approved through brainstorming
**Date:** 2026-07-28
**Source:** `SPEC.md` (authoritative for features, schema, APIs)
**Scope:** All three phases (CLI core, web + OPDS, sync + polish). Calibre import deferred to post-MVP.

This document defines *how* to implement what `SPEC.md` describes. Where
`SPEC.md` is the "what," this is the "how" — conventions, boundaries, and
decisions that fill the spec's gaps. It does not repeat the spec; it
supplements it.

---

## 0. Learning Project Principles

This is a guided learning project. The author is learning Go from scratch
but has 20 years of experience across C/C++/C#/JavaScript/Python.

**Implementation plan must follow these principles:**

- **Small step-by-step increments.** Each plan step should be a single
  focused concept or file — small enough to absorb in one sitting. Favor
  many small steps over few large ones.
- **Explain Go idioms, not programming basics.** Skip explanations of
  general concepts (types, functions, control flow, error handling as a
  concept). DO explain Go-specific things: goroutines, channels, interfaces,
  embedding, struct tags, `defer`, multiple return values, the `internal/`
  package convention, how `database/sql` connection pooling works, etc.
- **Call out Go-vs-other-language differences.** Where Go's approach differs
  notably from C/C++/C#/JS/Python (e.g., error handling via return values
  not exceptions, no classes/inheritance, implicit interfaces, package-level
  visibility via capitalization), note the contrast briefly.
- **Pause for questions.** Each step ends with an explicit invitation to
  ask questions before moving on. Don't rush ahead.
- **Show, don't just tell.** When introducing a Go pattern, show the actual
  code in context rather than abstract examples.
- **Reading real code counts.** Point to stdlib source or well-known Go
  project patterns when relevant — reading idiomatic Go is part of learning.

---

## 1. Approach: Sequential by Phase

Follow the spec's phase ordering. Build Phase 1 (CLI core) completely, then
Phase 2 (web + OPDS), then Phase 3 (sync + polish). Each phase produces a
working, testable deliverable:

- **Phase 1:** `incipit init`, `parse`, `lookup`, `add`, `add-user`,
  `list-users`, `remove-user` — a usable CLI library tool.
- **Phase 2:** `incipit serve` — browsable web library + OPDS catalog for
  KOReader.
- **Phase 3:** Reading progress sync, tag/series management, cover
  refinements, containerization, deployment.

All three phases are designed upfront in this doc so cross-cutting concerns
(auth, config, embed) are anticipated. Implementation plans are written
per-phase: Phase 1 plan first, then Phase 2 after Phase 1 is done, etc.

**Post-MVP (out of scope for initial plans):** `import-calibre`, thumbnail
generation, bulk operations, OpenSearch description refinements.

---

## 2. Package Boundaries and Interfaces

All app code lives under `internal/`. `models` is the shared type hub; every
other package imports it and nothing else does so transitively.

```
internal/
    models/      # Book, Tag, User, ReadingProgress, Metadata, LookupResult, SortTitle
    db/          # All SQL. Wraps *sql.DB. Typed methods take/return models structs.
    epub/        # Pure parsing: Parse(path) → Metadata, ParseOPF(io.Reader) → Metadata
    lookup/      # openlibrary + googlebooks clients, Merge(ol, gb), ParseResponse
    search/      # Searcher interface + LikeSearcher (FTS5Searcher upgrade in Phase 2)
    storage/     # Filesystem layout: files/{id}.epub, covers/{id}.jpg. MD5 hashing.
    opds/        # Feed/Entry structs, MarshalXML. Pure data → XML. No DB, no HTTP.
    server/      # Composition root: wires db + storage + lookup + opds + search into HTTP.
web/
    templates/   # Embedded via embed.FS
    static/      # Embedded via embed.FS
main.go         # Subcommand dispatch only. No business logic.
```

**Dependency rule (no cycles possible):**
- `models` depends on nothing.
- `db`, `epub`, `lookup`, `search`, `storage`, `opds` depend only on `models`
  (and stdlib).
- `server` depends on all of the above.
- `main.go` depends on `server` and the CLI-facing packages.

**Key boundary decisions:**
- `db` owns all SQL. No other package writes raw SQL. Typed methods
  (`InsertBook`, `ListBooks`, `GetProgress`) take/return `models` structs.
- `epub` is pure parsing — no DB, no HTTP, no file copying.
- `lookup` does not import `db`. Caching is handled by the caller (`add`
  command, `/api/lookup` handler) wrapping `lookup` calls with
  `db.CacheGet`/`db.CachePut`. The `lookup` clients expose
  `ParseResponse([]byte) (*Result, error)` so cached JSON is re-parsed
  identically to fresh responses.
- `storage` owns filesystem layout and file hashing. It receives book IDs
  from the caller — it does not own the DB.
- `opds` is pure XML generation from `models.Book`. No HTTP, no DB.
- `server` is the only package that knows about all dependencies. Auth
  middleware lives here.

---

## 3. Database Layer Conventions

**Migrations:** Versioned, embedded SQL. Schema lives in
`internal/db/migrations/` as numbered files (`001_init.sql`). Embedded at
build time via `embed.FS`. `Migrate()` tracks applied migrations in a
`schema_migrations` table and runs pending ones in a transaction. No
external migration tool — the binary is self-contained.

**`*DB` wrapper:** `internal/db` wraps `*sql.DB` in a `type DB struct`
holding the connection and exposing typed methods. Callers never touch
`database/sql` directly.

**Transactions:** Multi-step writes (e.g., add book + insert tags + cache
metadata) use `db.WithTx(ctx, func(tx *Tx) error)`. The `Tx` type exposes
the same methods as `DB` but on a transaction. Single reads/writes use `DB`
methods directly. No `BEGIN`/`COMMIT` in callers.

**Parameters:** `?` placeholders (SQLite standard). Named parameters
(`@isbn`, `@title`) via `sql.Named` for queries with many params. Never
string-concatenate values into SQL.

**Timestamps:** Schema uses `datetime('now')` for `added`/`updated` defaults.
The `updated` column must be explicitly set on each update — SQLite does
not auto-update it. The `db` layer handles this in every update method.

**PRAGMAs (set on open):**
- `PRAGMA journal_mode=WAL` (spec requirement, concurrent reads during
  writes).
- `PRAGMA foreign_keys=ON` (SQLite defaults to off; schema relies on
  `ON DELETE CASCADE`).

**Sort normalization:** `title_sort` and `author_sort` are normalized
versions (e.g., "The Expanse" → "Expanse, The"). Logic lives in
`internal/models` as `SortTitle(string) string` so it's testable in
isolation. The `db` layer calls it on insert/update. Rule: strip leading
articles ("The", "A", "An"), move to end with ", ".

---

## 4. EPUB Parsing and External Lookup

### EPUB parsing (`internal/epub`)

- `Parse(path string) (*models.Metadata, error)` — opens ZIP, reads
  `META-INF/container.xml` to find OPF path, parses OPF `<metadata>` block.
- `ParseOPF(io.Reader) (*models.Metadata, error)` — parses a bare OPF file
  (extracted from `Parse` for reuse by future Calibre import; called
  directly by `Parse` after ZIP extraction).
- **Namespace handling:** Dublin Core (`dc:title`, `dc:creator`) and OPF
  attributes (`opf:role`, `opf:scheme`) live in different namespaces. Define
  namespace URIs as constants; use `xml.Decoder` with namespace-aware
  parsing. No naive string matching.
- **ISBN extraction:** `<dc:identifier>` may be `urn:isbn:978...`, bare
  ISBN, or have `opf:scheme="ISBN"`. Strip `urn:isbn:` prefix. Normalize to
  digits-only (strip hyphens). If no ISBN, leave empty.
- **Multiple creators:** Use `opf:role` to distinguish: `role="aut"` is
  author. If no role attribute, take the first creator. Multiple authors
  with `role="aut"` → concatenate with ", ".

### Lookup clients (`internal/lookup`)

- Each client (`openlibrary`, `googlebooks`) implements a `Client` interface:
  `LookupByISBN(ctx, isbn) (*Result, error)` and
  `LookupByTitle(ctx, title, author) (*Result, error)`.
- Both set `User-Agent: incipit/0.1 (...)` and a 10-second timeout via
  `context.WithTimeout`.
- `lookup.Lookup(ctx, isbn, title, author)` orchestrates both clients and
  calls `Merge`.
- **Series extraction (Open Library):** Scan `subjects` array for entries
  matching `series:{name}`. Strip the `series:` prefix.
- **Offline graceful degradation:** If both APIs fail, return partial result
  (EPUB metadata) plus a non-fatal error. The `add` command proceeds with
  whatever metadata it has — never block adding a book on lookup failure.

### Merge functions (both pure, isolated, easy to tune later)

1. **Lookup merge:** `Merge(ol, gb *Result) *Result` — OL wins
   series/subjects/cover; GB wins rating/description/published date;
   first non-empty wins for title/author/pages/publisher.
2. **Metadata merge:** `models.MergeMetadata(epub Metadata, lookup
   LookupResult) Book` — where lookup-EPUB precedence lives. Default: lookup
   wins non-empty fields, EPUB fills gaps, subjects/tags merge as union.
   ISBN is the source of truth. Revisit exact precedence once we have real
   data — since it's a pure function with tests, changing rules is a
   single-file edit.

### Caching boundary

The `lookup` package does not import `db`. The caller wraps lookups with
cache get/put. Cache key is `(isbn, source)` per schema. On cache miss,
full JSON response is stored. The caller re-parses cached JSON via
`ParseResponse` — same path as fresh responses, no duplication.

---

## 5. HTTP Server, Auth, and Routing

**Composition root:** `server.New(cfg Config) *Server` constructs `db.DB`,
`storage.Storage`, `lookup` clients, and `search.Searcher`, stores them on
the `Server` struct, builds the chi router. `Run()` starts `http.Server`
with graceful shutdown on `SIGINT`/`SIGTERM` via `signal.NotifyContext`.

**Auth middleware — single global rule:** One middleware wraps the entire
router except `/health` and `/syncs/healthcheck`. It validates basic auth
against `db.GetUser(username)` and calls
`bcrypt.CompareHashAndPassword(hash, []byte(md5Password))`. On failure: 401
with `WWW-Authenticate: Basic` header. On success: injects authenticated
`*models.User` into `context.Context` so handlers access `user.ID` for
progress sync and write attribution. No toggle, no per-route conditional —
the two exempt routes mount separately outside the authed group.

**Password hashing chain:** KOReader MD5-hashes the password client-side
before sending it via basic auth. The server stores `bcrypt(md5(password))`.
CLI `add-user` takes plaintext → MD5-hash → bcrypt-hash → store.
Verification: the middleware extracts the password from the auth header
(which is already MD5-hashed by KOReader) and compares it against the
stored bcrypt hash via `bcrypt.CompareHashAndPassword`.

**Router structure:**
```
r := chi.NewRouter()
r.Use(middleware.RequestID, middleware.RealIP, requestLogger, recoverer)
r.Get("/health", s.health)               // no auth
r.Get("/syncs/healthcheck", s.syncHealth) // no auth
r.Group(func(r chi.Router) {
    r.Use(s.basicAuth)
    // /api/*, /opds/*, /, /book/*, /upload, /covers/*, /files/*, /syncs/*
})
```

**Request logging:** `log.Printf` with compact format (method, path,
status, duration, request ID). To stderr (scratch image has no journald).
No structured logging framework (not in dependency allowlist).

**Embed for scratch image:** `web/templates/` and `web/static/` are
embedded via `embed.FS`. Templates parsed once at startup with
`template.New().ParseFS(webFS, "templates/*.html")`. Static files served via
`http.FileServer(http.FS(webFS))` under `/static/`.

**Content-type correctness:**
- OPDS navigation feeds: `application/atom+xml; profile=opds-catalog; kind=navigation`
- OPDS acquisition feeds: `application/atom+xml; profile=opds-catalog; kind=acquisition`
- EPUB downloads: `application/epub+zip`
- Cover images: `image/jpeg`

Set explicitly in handlers — never guessed from file extensions.

---

## 6. OPDS Feed Generation and KOReader Sync

### OPDS (`internal/opds`)

- `Feed` and `Entry` structs with `encoding/xml` struct tags.
  `MarshalXML` produces valid Atom.
- `server` builds feeds by querying `db` and constructing `[]opds.Entry`
  from `[]models.Book`. `opds` never imports `db` or `server`.
- **Navigation feeds** (root, byauthor, byseries, bytag): entries link to
  sub-feeds, no download links. Content-type: `...; kind=navigation`.
- **Acquisition feeds** (newest, byauthor/{author}, byseries/{series},
  bytag/{tag}, search, book/{id}/download): entries include cover image
  link + acquisition link. Content-type: `...; kind=acquisition`.
- **Pagination:** 50 entries per feed. `?page=N` query param. Each feed
  includes `<link rel="next">` when more pages exist, `<link rel="self">`
  always. `db` query methods take `limit`/`offset`; handler computes offset
  from page.

### OPDS validator (test helper)

`opdstest.ValidateFeed(t, xmlBytes, assertions)` unmarshals feed XML and
asserts structure: required child elements (`id`, `title`, `updated`),
entry fields (`id`, `title`, `author`, links), correct `rel` and `type`
attributes on links, valid `urn:` ID format. Catches XML format bugs that
eyeballing curl output misses. Integration tests in `server` use it to
validate endpoint responses.

### Search (`internal/search`)

Modular searcher so different algorithms can be swapped in without touching
handlers or OPDS generation.

```go
type Searcher interface {
    Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error)
}
```

- **`LikeSearcher` (Phase 1):** `WHERE title LIKE ? OR author LIKE ?`. No
  setup needed. Ships immediately.
- **`FTS5Searcher` (Phase 2 upgrade):** SQLite FTS5 virtual table over
  title/author/series/description plus triggers to keep it in sync. BM25
  ranking. Drop-in swap behind same interface. `modernc.org/sqlite` includes
  FTS5 in the amalgamation.
- `server` receives a `search.Searcher` at construction — both
  `/api/books?q=` and `/opds/search?q=` route through the same searcher.
  Swapping implementations is a one-line change in `server.New`.
- **Constraint:** Whatever the searcher returns must map cleanly to
  `models.Book` so both JSON API and OPDS feed builders consume results
  without knowing which searcher produced them.
- Future search ideas (weighted fields, tag-aware filtering, series-aware
  ranking) are new files implementing `Searcher` — no handler changes.

### KOReader sync (`internal/server/sync.go`)

- `GET /syncs/healthcheck` → `{"state":"OK"}` (no auth, outside authed
  group).
- `GET /syncs/auth` → validates credentials (auth middleware already did
  this), returns `{"username":"...","role":"..."}`.
- `GET /syncs/progress/{document_hash}` → looks up by `(user_id,
  document_hash)`. Maps hash → `book_id` via `db.GetBookByFileHash`.
  Returns 404 if no progress.
- `PUT /syncs/progress/{document_hash}` → upserts progress for `(book_id,
  user_id)`. If no matching book (hash not in library), store with
  `book_id = NULL`.

### Schema adjustment for sync-by-hash

The spec's `reading_progress.book_id` is `NOT NULL` with a FK, but also
says the server can work "purely by hash without knowing which book it is."
These conflict. Resolution: make `book_id` nullable, drop the FK constraint
to allow NULL, keep the index. When a book with matching `file_hash` is
later added, a reconciliation pass or lazy lookup on `GET` can fill in
`book_id` retroactively. This is the one spec deviation, driven by a
spec-internal contradiction.

---

## 7. Testing Strategy

### Unit tests — per package, stdlib `testing`

- **`internal/epub`:** Fixture EPUBs in `testdata/` (single-author,
  multi-author, ISBN-with-scheme, no-ISBN). Parse fixtures, assert
  `Metadata` fields. No network, no DB.
- **`internal/lookup`:** Fixture JSON responses in `testdata/` (real OL and
  GB JSON snapshots). Parse via `ParseResponse`, assert `Result` fields.
  `Merge` tested as pure function with fixture pairs. `httptest.Server`
  serves fixtures for client tests — no real network.
- **`internal/models`:** `SortTitle` and `MergeMetadata` tested as pure
  functions with table-driven cases.
- **`internal/db`:** Each test opens real on-disk SQLite in `t.TempDir()`,
  runs migrations, exercises CRUD. Tests FK cascades, WAL mode,
  `datetime('now')` defaults. Real `modernc.org/sqlite`, no mocks.
- **`internal/search`:** `LikeSearcher` tested against real SQLite with
  seeded books. `FTS5Searcher` same, once implemented.
- **`internal/opds`:** `opdstest.ValidateFeed` parses XML, asserts
  structure. Unit tests build feeds from fixture `[]models.Book`.

### HTTP integration tests — `httptest` in `internal/server`

- `serverTest{}` helper boots a `Server` with temp-dir SQLite, temp-dir
  storage, and `httptest.Server`. Seeds books via the DB layer.
- Each test makes HTTP requests with `net/http`, asserts status, body,
  headers.
- **OPDS endpoints:** Assert content-type, parse XML with
  `opdstest.ValidateFeed`, assert entry counts, link `rel`/`type` attrs,
  download links resolve.
- **Auth:** Unauthenticated → 401. Authed → 200. Wrong password → 401.
  `/health` and `/syncs/healthcheck` → 200 unauthed.
- **Sync:** `PUT` progress then `GET` returns it. `GET` unknown hash →
  404. Progress keyed per user — user A not visible to user B.

### Test data discipline

- EPUB fixtures: committed to `internal/epub/testdata/`. Small, real EPUBs
  (public domain — e.g., Project Gutenberg).
- JSON fixtures: committed to `internal/lookup/testdata/`. Snapshots of
  real API responses, trimmed to relevant fields.
- No test touches the real network. All external calls go through
  `httptest` or fakes.

### CLI test seam

The `add` command's core logic lives in a function that takes dependencies
as interfaces, so tests inject fakes without spawning subprocesses. No
subprocess testing — it's slow and brittle.

### Verification commands

- `go test ./...` — all tests
- `go test ./internal/epub -run TestParse` — single package
- `go test ./internal/server -run TestOPDS -v` — single test
- `go vet ./...` — static analysis (must be clean)
- `gofmt -l .` — formatting check (must be empty)

---

## 8. CLI, Storage, and Build

### `main.go` subcommand dispatch

Minimal — parse `os.Args[1]`, route to a function. Each subcommand is a
top-level function. No framework. No business logic in `main.go` — each
function wires dependencies and calls into packages.

```go
switch cmd {
case "init":        runInit()
case "serve":       runServe()
case "parse":       runParse(os.Args[2])
case "lookup":      runLookup(os.Args[2:])
case "add":         runAdd(os.Args[2:])
case "add-user":    runAddUser(os.Args[2:])
case "list-users":  runListUsers()
case "remove-user": runRemoveUser(os.Args[2:])
default:            fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd); os.Exit(2)
}
```

`import-calibre` is post-MVP — not wired in initial implementation.

### Flag parsing

stdlib `flag` package. Each subcommand builds its own `flag.FlagSet` (e.g.,
`add` has `--no-lookup`, `--dry-run`). Unknown flag → print usage, exit 2.
Missing required arg → same.

### Config loading

`config.Load()` reads env vars (`INCIPIT_DB_PATH`, `INCIPIT_PORT`,
`INCIPIT_STORAGE_DIR`) with defaults. All subcommands needing DB/storage
call `config.Load()` first. No config file — env only (matches the spec's
k3s deployment model where config comes from pod env vars).

### Storage (`internal/storage`)

- `Storage` struct holds root dir path. Methods: `SaveBookFile(bookID,
  sourcePath)`, `SaveCover(bookID, imageData)`, `BookFilePath(bookID)`,
  `CoverPath(bookID)`.
- `HashFile(path) (string, error)` — MD5 of file content, returns hex.
  Called during `add`, stored in `books.file_hash`.
- Directories (`files/`, `covers/`) created lazily on first write via
  `os.MkdirAll`. No init step — works on a fresh PVC.
- Cover download: `lookup` returns cover URL; `add` command fetches via
  `net/http`, passes bytes to `storage.SaveCover`. Failure non-fatal —
  book added with `cover_path = NULL`.

### Embed for scratch image

```go
//go:embed web/templates web/static
var webFS embed.FS
```

Templates and static assets embedded into the binary. Runtime has no
filesystem for these — `scratch` image contains only the binary. Storage
dir (`/data`) is the only runtime filesystem, mounted from PVC.

### Dockerfile

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o incipit .

FROM scratch
COPY --from=builder /app/incipit /incipit
ENTRYPOINT ["/incipit"]
CMD ["serve"]
```

**Deliberate deviation from spec's Dockerfile:** The spec copies `web/`
into the runtime stage separately. Since we embed templates/static via
`go:embed`, only the binary is needed. Smaller image, simpler. This is a
conscious choice, not an accident.

### Quality gates (per phase completion)

- `go vet ./...` — clean
- `gofmt -l .` — empty
- `go test ./...` — passing

No CI pipeline configured yet — these are discipline enforced in the plan.

---

## 9. Cross-Phase Dependency Map

| Phase 2 needs from Phase 1 | Phase 3 needs from Phase 2 |
|---|---|
| `db` layer (all CRUD) | `server` with auth middleware |
| `epub.Parse` + `epub.ParseOPF` | OPDS feed generation |
| `lookup.Lookup` + `Merge` | `search.Searcher` interface |
| `storage` (save files/covers) | Web UI templates + upload |
| `models` types + `SortTitle` | JSON API endpoints |
| `config.Load` | Static file serving (embedded) |
| `MergeMetadata` | |

### What gets added per phase

- **Phase 1:** `db`, `epub` (incl. `ParseOPF`), `lookup`, `storage`,
  `models`, `search` (interface + `LikeSearcher`), `main.go` CLI dispatch,
  `init`/`parse`/`lookup`/`add`/`add-user`/`list-users`/`remove-user`
  subcommands.
- **Phase 2:** `server`, `opds`, web UI templates/static, `serve`
  subcommand, `search` upgrade to `FTS5Searcher` (optional, behind
  interface), JSON API endpoints.
- **Phase 3:** `sync.go` handlers, tag/series management UI, cover
  refinements, Dockerfile, deployment config.
- **Post-MVP:** `import-calibre`, thumbnail generation, bulk operations,
  OpenSearch description doc refinements.

`epub.ParseOPF` is extracted in Phase 1 as good separation anyway, so future
Calibre import is a drop-in with zero changes to the EPUB package.

---

## 10. Spec Deviations

Documented conscious choices where this design differs from `SPEC.md`:

1. **`reading_progress.book_id` nullable (Section 6).** Spec says `NOT NULL`
   with FK, but also says sync can work "purely by hash." Contradiction
   resolved by allowing NULL.
2. **Dockerfile drops `web/` copy (Section 8).** Spec copies `web/` into
   runtime stage; we embed via `go:embed` so only the binary is needed.
3. **Calibre import deferred to post-MVP (Section 1).** Spec lists it as
   Phase 3 (Step 13). Moved out of initial scope. `epub.ParseOPF` is
   extracted in Phase 1 to make the future addition a drop-in.
4. **Search algorithm not in spec (Section 6).** Spec defines `?q=`
   endpoints but not the search algorithm. This design adds a modular
   `Searcher` interface with `LikeSearcher` initially and `FTS5Searcher` as
   a documented upgrade path.
5. **Metadata merge precedence not in spec (Section 4).** Spec defines
   OL/GB lookup merge but not how `LookupResult` merges with EPUB
   `Metadata`. This design adds `models.MergeMetadata` as a pure function
   with a default (lookup wins non-empty, EPUB fills gaps, tags union) to
   be tuned after seeing real data.