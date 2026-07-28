# Incipit — Self-Hosted Ebook Server

A Go-based ebook server with EPUB management, OPDS catalog serving, reading
progress sync, and metadata lookup via Open Library and Google Books.

Designed for use with KOReader on Kobo e-readers, deployed on k3s.

---

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Database Schema](#database-schema)
4. [External APIs](#external-apis)
5. [Implementation Phases](#implementation-phases)
6. [API Reference](#api-reference)
7. [OPDS Catalog Spec](#opds-catalog-spec)
8. [KOReader Sync Protocol](#koreader-sync-protocol)
9. [Deployment](#deployment)
10. [Project Structure](#project-structure)

---

## Overview

Incipit is a single Go binary that serves as a personal ebook library server. It
replaces Calibre + Calibre-Web + KOReader sync server with one unified system.

**Core capabilities:**
- Upload EPUBs via web UI, extract metadata from the file
- Lookup enriched metadata from Open Library and Google Books
- Organize books by series, tags, author
- Serve catalog via OPDS for KOReader to browse and download
- Sync reading progress between KOReader devices
- Import existing Calibre libraries

**Design principles:**
- Single binary, single SQLite database, single container
- No external dependencies beyond the Go standard library + SQLite driver
- Pure-Go SQLite driver (no CGO) for easy cross-compilation
- All metadata lookups are optional and cached — works offline after first fetch
- Owns its own schema — not a Calibre compatibility layer

---

## Architecture

```
┌──────────────────────────────────────────────────────┐
│  incipit (single Go binary)                            │
│                                                      │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │ Web UI   │  │ OPDS     │  │ KOReader Sync   │   │
│  │ (HTML)   │  │ (XML)    │  │ (JSON)          │   │
│  └────┬─────┘  └────┬─────┘  └───────┬──────────┘   │
│       │             │                │              │
│  ┌────┴─────────────┴────────────────┴──────────┐   │
│  │              HTTP Router (chi)               │   │
│  └──────────────────────┬───────────────────────┘   │
│                         │                           │
│  ┌──────────┐  ┌───────┴───────┐  ┌────────────┐   │
│  │ EPUB     │  │ Lookup Service│  │ Storage    │   │
│  │ Parser   │  │ (OL + Google) │  │ (files)    │   │
│  └──────────┘  └───────────────┘  └────────────┘   │
│                         │                           │
│  ┌──────────────────────┴───────────────────────┐   │
│  │              SQLite (books.db)               │   │
│  └──────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────┘
         │              │                │
    ┌────┴────┐    ┌─────┴─────┐    ┌────┴─────┐
    │ Browser │    │ KOReader  │    │ k3s PVC  │
    │ (admin) │    │ (Kobo)    │    │ (storage)│
    └─────────┘    └───────────┘    └──────────┘
```

### Key Dependencies

| Dependency | Purpose | Why |
|------------|---------|-----|
| `modernc.org/sqlite` | SQLite driver | Pure Go, no CGO, easy cross-compile |
| `github.com/go-chi/chi/v5` | HTTP router | Lightweight, stdlib-compatible, supports middleware |
| `github.com/go-chi/cors` | CORS middleware | For web UI if you add a JS frontend later |
| Go stdlib only | Everything else | `net/http`, `encoding/xml`, `encoding/json`, `archive/zip`, `database/sql`, `html/template`, `image` |

---

## Database Schema

Single SQLite file at `/data/books.db`. Uses WAL mode for concurrent reads
during writes.

```sql
-- Core book record
CREATE TABLE books (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    title         TEXT NOT NULL,
    title_sort    TEXT,              -- normalized for sorting (e.g., "Expanse, The")
    author        TEXT NOT NULL,
    author_sort   TEXT,              -- "Corey, James S. A."
    series        TEXT,              -- "The Expanse"
    series_index  REAL,              -- 1, 1.5, 2, etc.
    isbn          TEXT,              -- "9780316129084"
    description   TEXT,
    publisher     TEXT,
    published     TEXT,              -- ISO date string: "2011-06-15"
    pages         INTEGER,
    rating        REAL,              -- 0-5, from Google Books
    cover_path    TEXT,              -- relative path: "covers/123.jpg"
    file_path     TEXT NOT NULL,     -- relative path: "files/123.epub"
    file_hash     TEXT,              -- MD5 of file content (for KOReader sync)
    file_size     INTEGER,           -- bytes
    added         TEXT DEFAULT (datetime('now')),
    updated       TEXT DEFAULT (datetime('now'))
);

-- Hierarchical tags (genres, subjects, custom categories)
CREATE TABLE tags (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    name      TEXT NOT NULL,
    parent_id INTEGER,              -- NULL = top-level tag
    FOREIGN KEY (parent_id) REFERENCES tags(id) ON DELETE CASCADE
);

CREATE TABLE book_tags (
    book_id INTEGER NOT NULL,
    tag_id  INTEGER NOT NULL,
    PRIMARY KEY (book_id, tag_id),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

-- Users (for web UI auth, KOReader sync auth, and library access)
-- Supports multiple users: family members, friends, etc.
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,    -- bcrypt hash of KOReader's MD5-hashed password
    role          TEXT DEFAULT 'user', -- 'admin' or 'user'
    created       TEXT DEFAULT (datetime('now'))
);

-- Reading progress (KOReader sync)
-- One position per (book, user) — latest writer wins, regardless of device.
-- The device field is informational only: "which device last saved".
CREATE TABLE reading_progress (
    book_id     INTEGER NOT NULL,
    user_id     INTEGER NOT NULL,
    percentage  REAL,
    progress    TEXT,               -- KOReader's XPath-like position string
    device      TEXT,               -- informational: "Kobo", "phone", etc.
    updated     TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (book_id, user_id),
    FOREIGN KEY (book_id) REFERENCES books(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Metadata lookup cache (avoid re-hitting APIs)
CREATE TABLE metadata_cache (
    isbn        TEXT,
    title       TEXT,
    author      TEXT,
    source      TEXT,               -- 'openlibrary' or 'googlebooks'
    response    TEXT,               -- full JSON response
    cached_at   TEXT DEFAULT (datetime('now')),
    PRIMARY KEY (isbn, source)
);

-- Indexes for common queries
CREATE INDEX idx_books_author ON books(author);
CREATE INDEX idx_books_series ON books(series);
CREATE INDEX idx_books_title_sort ON books(title_sort);
CREATE INDEX idx_books_added ON books(added);
CREATE INDEX idx_book_tags_tag ON book_tags(tag_id);
CREATE INDEX idx_reading_progress_book ON reading_progress(book_id);
CREATE INDEX idx_reading_progress_user ON reading_progress(user_id);
```

---

## External APIs

### Open Library (primary)

**No auth required.** Rate limit: 1 req/sec default, 3 req/sec with User-Agent.

**ISBN lookup:**
```
GET https://openlibrary.org/api/books?bibkeys=ISBN:{isbn}&format=json&jscmd=data
```

Response (simplified):
```json
{
  "ISBN:9780316129084": {
    "title": "Leviathan Wakes",
    "authors": [{"name": "James S. A. Corey"}],
    "publishers": [{"name": "Orbit Books"}],
    "publish_date": "2011-15-06",
    "number_of_pages": 577,
    "subjects": [
      {"name": "Space warfare"},
      {"name": "Fiction"},
      {"name": "series:The Expanse"}
    ],
    "cover": {
      "small": "https://covers.openlibrary.org/b/id/11295081-S.jpg",
      "medium": "https://covers.openlibrary.org/b/id/11295081-M.jpg",
      "large": "https://covers.openlibrary.org/b/id/11295081-L.jpg"
    }
  }
}
```

**Title/author search (when no ISBN):**
```
GET https://openlibrary.org/search.json?title={title}&author={author}&limit=5
```

Response (simplified):
```json
{
  "numFound": 42,
  "docs": [
    {
      "key": "/works/OL166894W",
      "title": "Leviathan Wakes",
      "author_name": ["James S. A. Corey"],
      "first_publish_year": 2011,
      "isbn": ["9780316129084"],
      "subject": ["Space warfare", "Fiction", "series:The Expanse"],
      "cover_i": 11295081,
      "number_of_pages_median": 577
    }
  ]
}
```

**Cover by ISBN (no API call needed, just construct URL):**
```
https://covers.openlibrary.org/b/isbn/{isbn}-L.jpg
```

**Series extraction:** Look for subjects matching `series:{name}`. The series
index is not provided by Open Library — you'd need to infer it or let the user
set it manually.

### Google Books (fallback/supplement)

**No auth for basic queries.** 1000 requests/day.

```
GET https://www.googleapis.com/books/v1/volumes?q=isbn:{isbn}
GET https://www.googleapis.com/books/v1/volumes?q=intitle:{title}+inauthor:{author}
```

Response (simplified):
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
        "thumbnail": "http://books.google.com/books/content?id=..."
      }
    }
  }]
}
```

**Series info:** Google Books does NOT provide series information reliably.
Use Open Library for series, Google for ratings and descriptions.

### Lookup Strategy

```
1. Extract ISBN from EPUB metadata
2. If ISBN present:
   a. Query Open Library by ISBN
   b. If incomplete, query Google Books by ISBN
   c. Merge results (OL for series/subjects, GB for rating/description)
3. If no ISBN:
   a. Query Open Library by title+author
   b. Query Google Books by title+author
   c. Merge results
4. If still no match:
   a. Return what was extracted from the EPUB file
   b. Let user fill in manually via web UI
5. Cache all API responses in metadata_cache table
```

---

## Implementation Phases

### Phase 1: Core (CLI only)

Build the data pipeline as command-line tools. No server, no web UI.
Each step is independently runnable and testable.

---

#### Step 1: Project Setup + Database

**Goal:** Go project that creates and initializes the SQLite database.

**Tasks:**
1. `go mod init github.com/jason/incipit` in `~/Repos/incipit`
2. Install dependencies: `go get modernc.org/sqlite`
3. Create package structure (see Project Structure section)
4. Implement `internal/db/db.go`:
   - `Open(path string) (*DB, error)` — opens SQLite, enables WAL mode
   - `Migrate()` — creates all tables if not present (raw SQL exec)
   - `Close()` — clean shutdown
5. Create `main.go` with subcommand dispatch:
   - `incipit init` — creates database at configured path
   - `incipit add-user --username X --password Y [--role admin]` — create user
     or reset password if user exists
   - `incipit list-users` — list all users
   - `incipit remove-user --username X` — delete user
   - `incipit serve` (placeholder, implemented in Phase 2)
6. Add a config struct (path to DB, port, storage dir) loaded from env vars
   or flags

**Learn:** Go modules, package structure, `database/sql` interface, error
handling patterns, subcommand dispatch.

**Deliverable:** `go run main.go init` creates `books.db` with all tables.

**Verify:** Open `books.db` in SQLite browser (or `sqlite3 books.db
.schema`) and see all tables.

---

#### Step 2: EPUB Parser

**Goal:** Parse metadata from an EPUB file.

**Tasks:**
1. Implement `internal/epub/epub.go`:
   - `type Metadata struct` with Title, Creator, Identifier, Language, Publisher, Date
   - `Parse(path string) (*Metadata, error)` — opens ZIP, finds OPF, parses XML
2. EPUB structure to handle:
   - EPUB is a ZIP file
   - Contains `META-INF/container.xml` which points to the OPF file path
   - OPF file contains `<metadata>` with Dublin Core elements
   - May have multiple `<dc:creator>` elements (use `opf:role` to distinguish)
   - `<dc:identifier>` may contain ISBN with scheme attribute
3. Add CLI command: `incipit parse <path>` — prints metadata as JSON

**Learn:** `archive/zip` reader, `encoding/xml` with namespace handling,
`io.Reader` patterns, struct tags for XML mapping.

**Deliverable:** `go run main.go parse ~/book.epub` prints:
```json
{
  "title": "Leviathan Wakes",
  "creator": "James S. A. Corey",
  "identifier": "urn:isbn:9780316129084",
  "language": "en",
  "publisher": "Orbit Books",
  "date": "2011-06-15"
}
```

**Verify:** Run against several real EPUBs from your collection. Check that
ISBN is extracted correctly (strip the `urn:isbn:` prefix).

**Key implementation detail — EPUB structure:**
```
book.epub (ZIP)
├── META-INF/
│   └── container.xml     ← points to OPF location
├── OEBPS/                (or content/, or root)
│   ├── content.opf       ← the metadata file
│   ├── toc.ncx
│   └── ...chapter files...
```

`container.xml` tells you where the OPF is:
```xml
<?xml version="1.0"?>
<container version="1.0">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
```

OPF metadata structure (simplified):
```xml
<package xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata>
    <dc:title>Leviathan Wakes</dc:title>
    <dc:creator>James S. A. Corey</dc:creator>
    <dc:identifier op:scheme="ISBN">9780316129084</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>Orbit Books</dc:publisher>
    <dc:date>2011-06-15</dc:date>
  </metadata>
</package>
```

---

#### Step 3: Open Library Lookup

**Goal:** Query Open Library by ISBN and get enriched metadata.

**Tasks:**
1. Implement `internal/lookup/openlibrary.go`:
   - `type Result struct` — Title, Author, Series, Subjects []string, CoverURL, Pages, Publisher, Published
   - `LookupByISBN(ctx context.Context, isbn string) (*Result, error)`
   - `LookupByTitle(ctx context.Context, title, author string) (*Result, error)`
2. Parse the Open Library JSON response:
   - Extract series from subjects: find `series:{name}` entries
   - Extract cover URL from `cover.large` or construct from `cover_i`
3. Set User-Agent header: `"incipit/0.1 (contact: your@email)"` for better rate limits
4. Add CLI command: `incipit lookup --isbn 9780316129084`
5. Add CLI command: `incipit lookup --title "Leviathan Wakes" --author "Corey"`

**Learn:** `net/http` client, `encoding/json`, `context.Context` for
timeouts, HTTP headers, rate limiting awareness.

**Deliverable:**
```bash
go run main.go lookup --isbn 9780316129084
```
```json
{
  "title": "Leviathan Wakes",
  "author": "James S. A. Corey",
  "series": "The Expanse",
  "subjects": ["Space warfare", "Fiction", "Interplanetary voyages"],
  "cover_url": "https://covers.openlibrary.org/b/id/11295081-L.jpg",
  "pages": 577,
  "publisher": "Orbit Books",
  "published": "2011"
}
```

**Verify:** Query several ISBNs from your collection. Confirm series info
extracted correctly.

**API details:**

ISBN lookup endpoint:
```
GET https://openlibrary.org/api/books?bibkeys=ISBN:{isbn}&format=json&jscmd=data
```

The response is a JSON object keyed by the bibkey:
```json
{
  "ISBN:9780316129084": { ... }
}
```

Title search endpoint:
```
GET https://openlibrary.org/search.json?title={title}&author={author}&limit=5
```

The response has `docs` array. Each doc has fields like `title`,
`author_name`, `isbn` (array), `subject` (array), `cover_i`, etc.

Series extraction: scan the `subjects` array (ISBN lookup) or `subject` array
(search) for entries matching the pattern `series:{name}`. Strip the
`series:` prefix.

---

#### Step 4: Google Books Fallback

**Goal:** Supplement Open Library data with Google Books ratings and
descriptions.

**Tasks:**
1. Implement `internal/lookup/googlebooks.go`:
   - `LookupByISBN(ctx context.Context, isbn string) (*Result, error)`
   - `LookupByTitle(ctx context.Context, title, author string) (*Result, error)`
2. Parse Google Books JSON response (different structure from Open Library)
3. Implement merge function:
   - `Merge(ol *Result, gb *Result) *Result`
   - Open Library wins for: series, subjects, cover (usually better)
   - Google Books wins for: rating, description, published date
   - First non-empty value wins for: title, author, pages, publisher
4. Update CLI: `incipit lookup` now tries Open Library first, then Google Books,
   then merges

**Learn:** Handling multiple API responses, data merging strategy, graceful
fallback when one source fails.

**Deliverable:**
```bash
go run main.go lookup --isbn 9780316129084
```
```json
{
  "title": "Leviathan Wakes",
  "author": "James S. A. Corey",
  "series": "The Expanse",
  "subjects": ["Space warfare", "Fiction"],
  "rating": 4.5,
  "description": "Two hundred years after migrating into space...",
  "cover_url": "https://covers.openlibrary.org/b/id/11295081-L.jpg",
  "pages": 577,
  "publisher": "Orbit Books",
  "published": "2011-06-15",
  "sources": ["openlibrary", "googlebooks"]
}
```

**Verify:** Find a book where Open Library has no rating but Google Books
does. Confirm the merge picks the right values.

---

#### Step 5: Add Book Command

**Goal:** End-to-end: parse EPUB, lookup metadata, store in DB, copy file to
storage.

**Tasks:**
1. Implement `internal/storage/storage.go`:
   - Stores EPUB files at `/data/files/{book_id}.epub`
   - Stores covers at `/data/covers/{book_id}.jpg`
   - `SaveBookFile(bookID int64, sourcePath string) error`
   - `SaveCover(bookID int64, imageData []byte) error`
2. Implement file hashing: MD5 of EPUB file content (for KOReader sync)
3. Implement `incipit add <path>` CLI command:
   - Parse EPUB metadata
   - Extract ISBN (strip `urn:isbn:` prefix)
   - Lookup metadata (Open Library + Google Books merge)
   - Display found metadata to user (stdout)
   - Insert book record into DB
   - Copy EPUB to storage dir
   - Download cover image if available
   - Print summary: "Added: {title} by {author}" + series if present
4. Add `--no-lookup` flag to skip API calls (use only EPUB metadata)
5. Add `--dry-run` flag to preview without saving

**Learn:** Tying multiple packages together, file I/O, database inserts,
user confirmation flow, graceful degradation (lookup fails → save what you
have).

**Deliverable:**
```bash
go run main.go add ~/Downloads/leviathan-wakes.epub
```
```
Parsing EPUB...
  Title: Leviathan Wakes
  Author: James S. A. Corey
  ISBN: 9780316129084

Looking up metadata...
  Found: Leviathan Wakes by James S. A. Corey
  Series: The Expanse
  Rating: 4.5/5
  Cover: [downloaded]

Saving...
  Book ID: 1
  File: /data/files/1.epub
  Cover: /data/covers/1.jpg

Added: Leviathan Wakes by James S. A. Corey (The Expanse #1)
```

**Verify:** Add several books. Check the DB has proper metadata. Check files
exist in storage. Check covers downloaded.

---

### Phase 2: Web Server + OPDS

Transform the CLI tool into a web server. Add the OPDS catalog so KOReader
can browse and download.

---

#### Step 6: HTTP Server Skeleton

**Goal:** Running HTTP server with routing and health check.

**Task:**
1. Install `go get github.com/go-chi/chi/v5`
2. Implement `internal/server/server.go`:
   - `Server` struct holding DB, storage, config
   - `New(cfg Config) *Server` constructor
   - `Run() error` — starts HTTP server with graceful shutdown
3. Routes (placeholder handlers returning 501):
   - `GET /health` → `{"status":"ok"}`
   - `GET /api/books` → list books (placeholder)
   - `GET /api/books/{id}` → book detail (placeholder)
   - `POST /api/books` → upload (placeholder)
   - `GET /opds` → OPDS root (placeholder)
   - `GET /` → web UI (placeholder)
4. Middleware: request logging, recovery from panics
5. Update `main.go`: `incipit serve` starts the server
6. Config from env vars: `INCIPIT_DB_PATH`, `INCIPIT_PORT`, `INCIPIT_STORAGE_DIR`

**Learn:** `net/http` server, chi router, middleware pattern, graceful
shutdown with `context`, signal handling (`os.Signal`), env var config.

**Deliverable:** `go run main.go serve` starts server on :8080. `curl
localhost:8080/health` returns `{"status":"ok"}`.

**Verify:** Hit each placeholder endpoint. Check server logs requests. Kill
with Ctrl-C, confirm graceful shutdown message.

---

#### Step 7: Book List + Detail API

**Goal:** JSON API for listing and viewing books.

**Tasks:**
1. Implement `internal/server/books.go`:
   - `GET /api/books` — paginated list with filters:
     - `?page=1&per_page=20`
     - `?series=The Expanse`
     - `?author=Corey`
     - `?tag=Science Fiction`
     - `?q=search+term` (title/author search)
     - `?sort=added|title|author|series`
   - Returns: `{ "books": [...], "total": 1100, "page": 1, "per_page": 20 }`
2. `GET /api/books/{id}` — full book detail including tags
3. `PUT /api/books/{id}` — update metadata (title, author, series,
   series_index, tags, description, rating)
4. `DELETE /api/books/{id}` — delete book + remove file + remove cover
5. `GET /api/tags` — list all tags (hierarchical)
6. `GET /api/series` — list all series with book counts

**Learn:** REST API patterns, JSON request/response, SQL query building
(with parameters, not string concatenation), pagination, HTTP status codes.

**Deliverable:**
```bash
curl localhost:8080/api/books?series=The+Expanse
```
```json
{
  "books": [
    {"id": 1, "title": "Leviathan Wakes", "author": "James S. A. Corey",
     "series": "The Expanse", "series_index": 1, "cover": "/covers/1.jpg"},
    {"id": 2, "title": "Caliban's War", "author": "James S. A. Corey",
     "series": "The Expanse", "series_index": 2, "cover": "/covers/2.jpg"}
  ],
  "total": 9,
  "page": 1,
  "per_page": 20
}
```

**Verify:** Use `curl` or a REST client to test all endpoints. Add books via
CLI first, then query via API.

---

#### Step 8: Web UI

**Goal:** HTML pages for browsing and managing the library.

**Tasks:**
1. Create `web/templates/` directory with:
   - `base.html` — common layout (header, nav, footer)
   - `index.html` — book grid with covers, pagination, search bar
   - `book.html` — book detail page with editable form
   - `upload.html` — file upload form
   - `login.html` — basic auth login page
2. Create `web/static/style.css` — clean, minimal, readable. No framework.
3. Implement `internal/server/web.go`:
   - `GET /` — render index.html with book list
   - `GET /book/{id}` — render book.html with detail
   - `GET /upload` — render upload form
   - `GET /covers/{id}.jpg` — serve cover images from storage
   - `GET /files/{id}.epub` — serve EPUB files (auth required)
4. Template rendering with `html/template`:
   - `template.Must(template.New().ParseGlob("web/templates/*.html"))`
   - Pass data structs to templates
5. Keep it simple: server-rendered HTML, no SPA, no JS framework. Maybe a
   tiny bit of vanilla JS for search and tag editing.

**Learn:** Go's `html/template` (auto-escaping, template inheritance),
serving static files, form handling, file serving with proper content types.

**Deliverable:** Open `http://localhost:8080/` in browser. See book grid
with covers. Click a book → detail page. Upload page works.

**Verify:** Upload a new EPUB via the web UI. See it appear in the list. Click
it. Edit its metadata. Confirm changes saved.

---

#### Step 9: OPDS Catalog

**Goal:** Serve OPDS feeds for KOReader to browse and download books.

**Tasks:**
1. Implement `internal/opds/opds.go`:
   - `type Feed struct` with Entries
   - `type Entry struct` — title, author, id, links, content
   - `MarshalXML() ([]byte, error)` — generate valid OPDS Atom XML
2. Endpoints:
   - `GET /opds` — root catalog (navigation feed)
     - Links to: Newest, By Author, By Title, By Series, By Tag, Search
   - `GET /opds/newest` — acquisition feed, 50 newest books
   - `GET /opds/byauthor` — navigation feed: list of authors
   - `GET /opds/byauthor/{author}` — acquisition feed: books by that author
   - `GET /opds/byseries` — navigation feed: list of series
   - `GET /opds/byseries/{series}` — acquisition feed: books in series,
     ordered by series_index
   - `GET /opds/bytag` — navigation feed: tag tree
   - `OPDS/search?q={query}` — search results feed
   - `GET /opds/book/{id}/download` — serve the EPUB file
3. OPDS feed structure (Atom XML):
   ```xml
   <feed xmlns="http://www.w3.org/2005/Atom">
     <id>urn:incipit:newest</id>
     <title>Newest Books</title>
     <updated>2024-01-15T12:00:00Z</updated>
     <link rel="self" href="/opds/newest" type="application/atom+xml"/>
     <link rel="next" href="/opds/newest?page=2" type="application/atom+xml"/>

     <entry>
       <id>urn:incipit:book:1</id>
       <title>Leviathan Wakes</title>
       <author><name>James S. A. Corey</name></author>
       <category term="The Expanse" label="series"/>
       <content type="text">Two hundred years after migrating...</content>
       <link rel="http://opds-spec.org/image" href="/covers/1.jpg"
             type="image/jpeg"/>
       <link rel="http://opds-spec.org/acquisition"
             href="/opds/book/1/download" type="application/epub+zip"/>
     </entry>
   </feed>
   ```
4. Pagination: 50 entries per feed, `<link rel="next">` for next page
5. Content type negotiation:
   - OPDS feeds: `application/atom+xml; profile=opds-catalog`
   - Navigation feeds: `application/atom+xml; profile=opds-catalog; kind=navigation`
   - Acquisition feeds: `application/atom+xml; profile=opds-catalog; kind=acquisition`
6. Test with KOReader's OPDS browser

**Learn:** XML generation in Go (`encoding/xml` marshaler), Atom spec, OPDS
spec extensions, content negotiation, pagination in XML.

**Deliverable:** Point KOReader's OPDS browser at `http://your-server:8080/opds`.
Browse library by series/author/tag. Download a book. Read it.

**Verify:** This is the big milestone. Get your Kobo browsing and downloading
from your server.

**OPDS reference:** The spec is at https://specs.opds.io/opds-catalog-1-2
but you don't need to read the whole thing. The structure above is what
KOReader expects. Look at Calibre's OPDS output for a reference
implementation: start Calibre Content Server, visit `http://localhost:8080/opds`
and inspect the XML.

---

### Phase 3: Sync + Polish

Add reading progress sync and finalize for deployment.

---

#### Step 10: KOReader Progress Sync

**Goal:** Implement the KOReader sync protocol for reading position.

**Tasks:**
1. Implement `internal/server/sync.go`:
   - `GET /syncs/healthcheck` → `{"state":"OK"}` (for the sync server health check)
   - `GET /syncs/auth` → validate basic auth credentials, return user info
   - `PUT /syncs/progress/{document_hash}` — save reading position
     - Body: `{"percentage": 0.318, "progress": "/body/DocFragment[20]...", "device": "Kobo"}`
     - Requires basic auth
   - `GET /syncs/progress/{document_hash}` — get latest reading position
     - Returns the same body, or 404 if no progress saved
     - Requires basic auth
   - No `POST /syncs/register` endpoint — users are created via CLI only
2. The `document_hash` is the MD5 of the EPUB file content. Map it to a book
   in your DB via `books.file_hash` (optional — the sync server can work
   purely by hash without knowing which book it is).
3. Store progress in `reading_progress` table, keyed by `(book_id, user_id)`.
   If no matching book in the library, store by hash only (book_id = NULL
   possible, or use a separate hash-only progress table — simpler to just
   require the book be in the library).
4. Authentication: basic auth over HTTPS. Validate against `users` table.
5. User management via CLI:
   - `incipit add-user --username jason --password '...'`
   - If user already exists, updates their password (acts as password reset)
   - `incipit add-user --username sarah --password '...' --role admin`
   - `incipit list-users`
   - `incipit remove-user --username sarah`

**Auth model — all endpoints require authentication:**

Simpler and safer. No toggle, no conditional logic. Every endpoint requires
basic auth. KOReader's OPDS browser and sync plugin both support basic auth
natively — you enter credentials once on each device.

| Surface | Auth? | Why |
|---------|-------|-----|
| `GET /opds/*` | **Yes** | Authenticate to browse catalog |
| `GET /covers/*` | **Yes** | Authenticated image serving |
| `GET /opds/book/{id}/download` | **Yes** | Authenticate to download |
| `GET /` (web UI) | **Yes** | Login required to browse |
| `POST /upload` | **Yes** | Write operation |
| `PUT /api/books/{id}` | **Yes** | Write operation |
| `DELETE /api/books/{id}` | **Yes** | Destructive |
| `PUT /syncs/progress/*` | **Yes** | Needs user identity |
| `GET /syncs/progress/*` | **Yes** | Same user's data |
| `GET /syncs/auth` | **Yes** | Validates credentials for KOReader config |
| `GET /health` | No | Health check for k8s probes (no auth) |
| `GET /syncs/healthcheck` | No | KOReader sync health check (no auth) |

The only unauthenticated endpoints are health checks (needed for k8s
readiness/liveness probes and KOReader's initial connection test). Everything
else requires basic auth credentials.

**Learn:** Basic auth implementation, bcrypt password hashing, JSON API,
understanding an external protocol by reading its docs.

**Deliverable:** Configure KOReader's Progress Sync plugin to point at your
server. Read on Kobo, check that position syncs. Open on another device (e.g.,
KOReader on desktop), confirm it picks up the position.

**Verify:** Read a few pages on the Kobo. Open KOReader on your desktop. The
"Progress sync" plugin should pull the position and ask if you want to jump
to it.

---

#### Step 11: Series + Tags Management

**Goal:** Full CRUD for series and hierarchical tags via web UI.

**Tasks:**
1. Tag management page `/tags`:
   - List all tags as a tree (parent/child indentation)
   - Create new tag (with optional parent)
   - Rename tag
   - Delete tag (with confirmation)
   - Merge tags (rename one to match another)
2. Book editing page `/book/{id}/edit`:
   - Edit all metadata fields
   - Add/remove tags (multi-select)
   - Set series + series_index
   - Upload new cover image
   - Delete book (with confirmation)
3. Series overview page `/series`:
   - List all series with book counts
   - Click a series → see all books in order
   - Edit series name (applies to all books in series)
4. Bulk operations:
   - Select multiple books → assign tag
   - Select multiple books → set series
   - Select multiple books → delete

**Learn:** CRUD forms, hierarchical data display, transaction safety (rename
a series updates all books), confirmation dialogs in HTML forms.

**Deliverable:** Open the web UI, create a "Science Fiction > Space Opera"
hierarchical tag, assign it to books. Browse by that tag in OPDS — see the
hierarchy reflected.

**Verify:** Create tags, assign them, verify they appear in OPDS bytag feed.
Rename a series, verify all books in that series update.

---

#### Step 12: Covers + Image Handling

**Goal:** Efficient cover serving, downloading, and thumbnails.

**Tasks:**
1. Cover serving endpoint `GET /covers/{id}.jpg`:
   - Serve from storage directory
   - Set proper content-type and cache headers (`Cache-Control: max-age=31536000`)
   - 404 if no cover (serve a placeholder image)
2. Cover download during add-book:
   - Download from Open Library URL
   - Save to `/data/covers/{book_id}.jpg`
   - If no cover URL, leave `cover_path` NULL
3. Cover upload via web UI:
   - `POST /api/books/{id}/cover` with multipart upload
   - Save image to storage, update `cover_path` in DB
4. Thumbnail generation (optional):
   - Generate small (100px), medium (200px), large (full) variants
   - Use Go's `image` package for resizing
   - Serve at `/covers/{id}-small.jpg`, `/covers/{id}-medium.jpg`
5. OPDS cover links:
   - Include thumbnail in OPDS entries
   - Include full cover link

**Learn:** Image processing with Go stdlib, HTTP cache headers, file serving
with `http.ServeFile`, multipart upload handling.

**Deliverable:** Covers appear in web UI and in KOReader's OPDS browser.
Uploading a custom cover works.

**Verify:** Browse OPDS on Kobo — covers should show next to book entries.

---

#### Step 13: Import from Calibre

**Goal:** Import an existing Calibre library, preserving all metadata.

**Tasks:**
1. CLI command: `incipit import-calibre <calibre-library-dir>`
2. Walk the Calibre library directory structure:
   ```
   /Calibre Library/
     Author Name/
       Book Title (123)/
         metadata.opf          ← metadata in Dublin Core XML
         cover.jpg
         Book Title - Author.epub
   ```
3. For each book folder:
   - Parse `metadata.opf` — same Dublin Core format as EPUB OPF
   - Extract: title, author, series, series_index, tags, isbn, rating,
     publisher, published date, comments/description
   - Copy the EPUB file to incipit storage
   - Copy `cover.jpg` if present
   - Compute MD5 hash of the EPUB
   - Insert book record with all metadata
4. Progress reporting: print "[42/1100] Importing: Leviathan Wakes..."
5. Duplicate detection: skip if file_hash already in DB
6. Rate limiting: process sequentially, no API calls (metadata comes from
   OPF files, not external APIs)
7. Tag import: Calibre tags → incipit tags (flat, no hierarchy unless you want
   to map Calibre's hierarchical tags)
8. Handle Calibre's custom columns (if used):
   - Check for `#genre` or similar in OPF
   - Import as tags

**Calibre OPF format** (same Dublin Core you already parse in Step 2):
```xml
<?xml version='1.0' encoding='utf-8'?>
<package xmlns:dc="http://purl.org/dc/elements/1.1/">
  <metadata>
    <dc:title>Leviathan Wakes</dc:title>
    <dc:creator opf:file-as="Corey, James S. A.">James S. A. Corey</dc:creator>
    <dc:identifier opf:scheme="ISBN">9780316129084</dc:identifier>
    <dc:language>en</dc:language>
    <dc:publisher>Orbit Books</dc:publisher>
    <dc:date>2011-06-15</dc:date>
    <dc:subject>Science Fiction</dc:subject>
    <dc:subject>Space Opera</dc:subject>
    <meta name="calibre:series" content="The Expanse"/>
    <meta name="calibre:series_index" content="1"/>
    <meta name="calibre:rating" content="8"/>
    <meta name="calibre:comments" content="Two hundred years after..."/>
  </metadata>
</package>
```

**Learn:** Filesystem traversal (`filepath.WalkDir`), batch processing,
progress reporting, duplicate detection, parsing slightly different XML
format (Calibre uses `<meta>` tags for series/rating, not standard Dublin
Core).

**Deliverable:** `go run main.go import-calibre ~/Calibre\ Library/` imports
all 1100 books with metadata, covers, tags, series info.

**Verify:** Compare book count before and after. Spot-check a few books in
the web UI — series, tags, cover should match Calibre. Check a book's
file_hash matches what KOReader computes (for sync to work, the EPUB must be
identical or have same content MD5).

---

#### Step 14: Containerize + Deploy to k3s

**Goal:** Deploy incipit as a container on your k3s cluster with PVC and
ingress.

**Tasks:**
1. Create `Dockerfile` (multi-stage build):
   ```dockerfile
   # Build stage
   FROM golang:1.22-alpine AS builder
   WORKDIR /app
   COPY go.mod go.sum ./
   go mod download
   COPY . .
   CGO_ENABLED=0 go build -o incipit .

   # Runtime stage
   FROM scratch
   COPY --from=builder /app/incipit /incipit
   COPY --from=builder /app/web /web
   ENTRYPOINT ["/incipit"]
   CMD ["serve"]
   ```
2. Create Helm values for your existing `veridian-apps` chart:
   - Image: `ghcr.io/jason/incipit:latest` (or your registry)
   - PVC for `/data` (database, files, covers)
   - Service on port 8080
   - Ingress with your Traefik setup (TLS, hostname like `incipit.yourdomain.com`)
   - Environment variables for config
3. Add a health check (readiness probe) hitting `GET /health`
4. Add a liveness probe hitting `GET /health`
5. PVC backup strategy (the database + files are all in `/data`)
6. Create a simple GitHub Actions workflow (optional) for CI/CD:
   - Build Docker image on push to main
   - Push to your registry
   - Update Helm values with new tag

**Learn:** Multi-stage Docker builds, scratch images (zero OS, just the
binary), Helm chart integration, k3s deployment patterns.

**Deliverable:** `kubectl apply` deploys incipit. Browse to
`https://incipit.yourdomain.com`. KOReader connects to
`https://incipit.yourdomain.com/opds`. Sync works over HTTPS.

**Verify:** Full end-to-end on the cluster:
1. Deploy
2. Import Calibre library via `kubectl exec` running the import command
3. Browse OPDS from Kobo
4. Download a book
5. Read, sync progress
6. Check progress synced back

---

## API Reference

### Web UI Endpoints (HTML)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/` | Book list (grid view, paginated, searchable) |
| GET | `/book/{id}` | Book detail page |
| GET | `/book/{id}/edit` | Edit metadata form |
| POST | `/book/{id}/edit` | Save metadata changes |
| GET | `/upload` | Upload form |
| POST | `/upload` | Handle EPUB upload |
| GET | `/tags` | Tag management tree |
| GET | `/series` | Series list with counts |
| GET | `/covers/{id}.jpg` | Serve cover image |
| GET | `/files/{id}.epub` | Download EPUB (auth required) |

### JSON API Endpoints

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/health` | Health check |
| GET | `/api/books` | List books (paginated, filterable) |
| GET | `/api/books/{id}` | Book detail (JSON) |
| POST | `/api/books` | Upload new book |
| PUT | `/api/books/{id}` | Update book metadata |
| DELETE | `/api/books/{id}` | Delete book |
| GET | `/api/tags` | List all tags |
| POST | `/api/tags` | Create tag |
| PUT | `/api/tags/{id}` | Rename/reparent tag |
| DELETE | `/api/tags/{id}` | Delete tag |
| GET | `/api/series` | List all series |
| POST | `/api/series` | Create/rename series |
| GET | `/api/lookup` | Lookup metadata by ISBN or title+author |

### OPDS Endpoints (XML)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/opds` | Root catalog (navigation) |
| GET | `/opds/newest` | Newest books (acquisition) |
| GET | `/opds/byauthor` | Author list (navigation) |
| GET | `/opds/byauthor/{author}` | Books by author (acquisition) |
| GET | `/opds/byseries` | Series list (navigation) |
| GET | `/opds/byseries/{series}` | Books in series (acquisition) |
| GET | `/opds/bytag` | Tag tree (navigation) |
| GET | `/opds/bytag/{tag}` | Books with tag (acquisition) |
| GET | `/opds/search?q={query}` | Search results (acquisition) |
| GET | `/opds/book/{id}/download` | Download EPUB file |

### KOReader Sync Endpoints (JSON)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/syncs/healthcheck` | Sync server health (no auth) |
| GET | `/syncs/auth` | Validate credentials, return user info |
| GET | `/syncs/progress/{hash}` | Get reading position for a document |
| PUT | `/syncs/progress/{hash}` | Save reading position for a document |

No `POST /syncs/register` endpoint — users are created via CLI only.

### CLI User Management

| Command | Purpose |
|---------|---------|
| `incipit add-user --username X --password Y` | Create user or reset password if exists |
| `incipit add-user --username X --password Y --role admin` | Create admin user |
| `incipit list-users` | List all users |
| `incipit remove-user --username X` | Delete user and their progress |

---

## OPDS Catalog Spec

### Navigation Feed (lists categories)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <id>urn:incipit:root</id>
  <title>Incipit Library</title>
  <updated>2024-07-28T12:00:00Z</updated>
  <author>
    <name>Incipit</name>
    <uri>https://incipit.yourdomain.com</uri>
  </author>
  <link rel="self" href="/opds" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  <link rel="start" href="/opds" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>

  <entry>
    <title>Newest Books</title>
    <link rel="subsection" href="/opds/newest" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  </entry>
  <entry>
    <title>By Author</title>
    <link rel="subsection" href="/opds/byauthor" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  </entry>
  <entry>
    <title>By Series</title>
    <link rel="subsection" href="/opds/byseries" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  </entry>
  <entry>
    <title>By Tag</title>
    <link rel="subsection" href="/opds/bytag" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  </entry>
  <entry>
    <title>Search</title>
    <link rel="search" href="/opds/search?q={searchTerms}" type="application/atom+xml; profile=opds-catalog; kind=navigation"/>
  </entry>
</feed>
```

### Acquisition Feed (lists books to download)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <id>urn:incipit:newest</id>
  <title>Newest Books</title>
  <updated>2024-07-28T12:00:00Z</updated>
  <link rel="self" href="/opds/newest" type="application/atom+xml; profile=opds-catalog; kind=acquisition"/>
  <link rel="next" href="/opds/newest?page=2" type="application/atom+xml; profile=opds-catalog; kind=acquisition"/>

  <entry>
    <id>urn:incipit:book:1</id>
    <title>Leviathan Wakes</title>
    <author><name>James S. A. Corey</name></author>
    <category term="The Expanse" label="series"/>
    <category term="Science Fiction" label="tag"/>
    <content type="text">Two hundred years after migrating into space...</content>
    <link rel="http://opds-spec.org/image" href="/covers/1.jpg" type="image/jpeg"/>
    <link rel="http://opds-spec.org/acquisition" href="/opds/book/1/download" type="application/epub+zip"/>
    <published>2024-07-20T10:00:00Z</published>
  </entry>
</feed>
```

---

## KOReader Sync Protocol

The KOReader sync protocol is a simple key-value store. The server does not
interpret the progress data — it stores and returns it as-is.

### User Management (CLI only)

No self-service registration endpoint. Users are created via CLI by an admin:

```bash
incipit add-user --username jason --password 'secret'
# If user already exists, updates their password (acts as password reset)
# Use --role admin for admin users:
incipit add-user --username sarah --password 'secret' --role admin
incipit list-users
incipit remove-user --username sarah
```

KOReader MD5-hashes the password client-side before sending it to the server.
The server stores a bcrypt hash of that MD5 hash. This means:
- The plaintext password never leaves the device
- The server never sees the original password
- If the DB leaks, attackers get bcrypt(MD5(password)), which is still hard to crack

When creating a user via CLI, you provide the plaintext password. The CLI
tool MD5-hashes it (matching what KOReader sends), then bcrypt-hashes that
for storage. Alternatively, accept the pre-MD5'd password directly via a
`--password-hash` flag for scripted setups.

### Authenticate

```
GET /syncs/auth
Authorization: Basic base64(username:md5_password)
```
Response: `200 OK` with user info JSON
```json
{"username": "jason", "role": "admin"}
```

KOReader sends this on startup to verify credentials. If it fails, KOReader
shows an auth error.

### Save progress

```
PUT /syncs/progress/{document_hash}
Authorization: Basic base64(username:md5_password)
Content-Type: application/json

{
  "percentage": 0.318,
  "progress": "/body/DocFragment[20]/body/p[22]/img.0",
  "device": "Kobo"
}
```
Response: `200 OK`

### Get progress

```
GET /syncs/progress/{document_hash}
Authorization: Basic base64(username:md5_password)
```
Response: `200 OK`
```json
{
  "percentage": 0.318,
  "progress": "/body/DocFragment[20]/body/p[22]/img.0",
  "device": "Kobo"
}
```

If no progress saved for this (user, document_hash), return `404 Not Found`.

### How sync works

The `document_hash` is an MD5 of the document content. KOReader computes this
automatically. Your `books.file_hash` field stores the same MD5 — so you can
map a sync request to a specific book (optional — the sync server can work
purely by hash without knowing which book it is).

Progress is stored per `(book_id, user_id)`. Latest writer wins:
1. Kobo saves at 30% → overwrites the row
2. Phone checks → gets 30%
3. Phone reads to 35%, saves → overwrites the row
4. Kobo checks → gets 35%

The `device` field is informational only — "which device last saved". The
server doesn't use it for storage logic. You could display it in the web UI
("last read on Kobo at 31%").

### Health check

```
GET /syncs/healthcheck
```
Response: `200 OK`
```json
{"state": "OK"}
```

KOReader's sync plugin checks this on first connection to verify the server
is reachable.

---

## Deployment

### Container

Multi-stage Docker build produces a `FROM scratch` image containing only:
- The `incipit` binary
- The `web/templates/` and `web/static/` directories

Image size: ~15-20MB.

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o incipit .

FROM scratch
COPY --from=builder /app/incipit /incipit
COPY --from=builder /app/web /web
ENTRYPOINT ["/incipit"]
CMD ["serve"]
```

### k3s Deployment

Uses your existing `veridian-apps` Helm chart pattern:

```yaml
# values/incipit.yaml
image:
  repository: ghcr.io/jason/incipit
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
```

### Storage Layout on PVC

```
/data/
  books.db               ← SQLite database
  files/
    1.epub
    2.epub
    ...
  covers/
    1.jpg
    2.jpg
    ...
```

### Backup

The entire system state is in `/data/` — one database file plus book files and
covers. Backup is `rsync` or `restic` of that directory. No complex state
elsewhere.

---

## Project Structure

```
~/Repos/incipit/
├── go.mod
├── go.sum
├── main.go                       # Entry point with subcommand dispatch
├── internal/
│   ├── db/
│   │   ├── db.go                 # Open, Migrate, Close
│   │   ├── books.go              # Book CRUD queries
│   │   ├── tags.go              # Tag CRUD queries
│   │   └── progress.go           # Reading progress queries
│   ├── epub/
│   │   └── epub.go              # EPUB parsing (zip + XML)
│   ├── lookup/
│   │   ├── openlibrary.go       # Open Library API client
│   │   ├── googlebooks.go       # Google Books API client
│   │   └── merge.go             # Merge results from multiple sources
│   ├── models/
│   │   └── models.go            # Shared types (Book, Tag, etc.)
│   ├── opds/
│   │   └── opds.go              # OPDS XML feed generation
│   ├── server/
│   │   ├── server.go            # HTTP server setup, routing, middleware
│   │   ├── books.go             # Book API handlers
│   │   ├── web.go               # HTML page handlers
│   │   ├── sync.go              # KOReader sync handlers
│   │   └── upload.go            # EPUB upload handler
│   └── storage/
│       └── storage.go           # File storage (books, covers)
├── web/
│   ├── templates/
│   │   ├── base.html
│   │   ├── index.html
│   │   ├── book.html
│   │   ├── upload.html
│   │   └── edit.html
│   └── static/
│       └── style.css
├── Dockerfile
└── SPEC.md                       # This file
```

### Go Module Path

```
module github.com/jason/incipit

go 1.22
```

Use this module path in `go.mod`. If you push to GitHub later, it'll work
without changes. If you use a different VCS, adjust accordingly.

---

## Timeline

### Week 1: Go Fundamentals + EPUB Parsing

| Day | Step | Focus | Outcome |
|-----|------|-------|---------|
| 1   | 1    | Project setup, DB schema, Go modules | `incipit init` creates DB |
| 2-3 | 2   | EPUB parsing, ZIP/XML, struct tags | `incipit parse book.epub` works |
| 4   | 3    | Open Library lookup, HTTP client, JSON | `incipit lookup --isbn X` works |
| 5   | 3-4  | Google Books fallback, merge strategy | Multi-source lookup works |
| 6-7 | 5    | Add book command, file storage, covers | `incipit add book.epub` works end-to-end |

### Week 2: Web Server + OPDS

| Day | Step | Focus | Outcome |
|-----|------|-------|---------|
| 8   | 6    | HTTP server, chi router, middleware | `incipit serve` running |
| 9   | 7    | JSON API for books, pagination, filtering | `/api/books` works |
| 10  | 8    | HTML templates, web UI, file serving | Browse library in browser |
| 11  | 8    | Upload form, cover serving, polish | Upload via web works |
| 12-13 | 9  | OPDS feeds, XML generation, pagination | KOReader browses catalog |
| 14  | 9    | Test with Kobo, fix OPDS issues | **Kobo downloads books** |

### Week 3: Sync + Management

| Day | Step | Focus | Outcome |
|-----|------|-------|---------|
| 15  | 10   | KOReader sync protocol, auth, bcrypt | Reading position syncs |
| 16  | 10   | Test sync between Kobo + desktop KOReader | **Sync works** |
| 17  | 11   | Tag management UI, hierarchical tags | Create/assign tags in UI |
| 18  | 11   | Series management, bulk operations | Full library management |
| 19  | 12   | Covers, thumbnails, cache headers | Covers in OPDS + web UI |
| 20  | 12   | Cover upload, placeholder covers | Cover management complete |
| 21  | 13   | Calibre OPF import, batch processing | Import 1100 books |

### Week 4: Deploy + Polish

| Day | Step | Focus | Outcome |
|-----|------|-------|---------|
| 22  | 13   | Test import, verify metadata integrity | All 1100 books imported |
| 23  | 14   | Dockerfile, multi-stage build, scratch image | Container builds |
| 24  | 14   | Helm values, PVC, ingress, k3s deploy | Running on cluster |
| 25  | 14   | Full end-to-end test on k3s | Everything works in prod |
| 26  | -    | Bug fixes, edge cases, error handling | Robust |
| 27  | -    | Documentation, README, usage guide | Shareable |
| 28  | -    | Final polish, performance check | Done |

---

## Key Learning Resources

### Go fundamentals (Steps 1-2)
- [A Tour of Go](https://go.dev/tour/) — interactive basics
- [Go by Example](https://gobyexample.com/) — snippets for common patterns
- [Effective Go](https://go.dev/doc/effective_go) — idioms and conventions
- `go doc` command — read docs in terminal for any package

### HTTP + APIs (Steps 3-7)
- [Go HTTP server patterns](https://go.dev/doc/articles/http-handlers)
- chi router docs: https://pkg.go.dev/github.com/go-chi/chi/v5
- [Context package](https://pkg.go.dev/context) — read the examples

### XML + OPDS (Step 9)
- `encoding/xml` docs: https://pkg.go.dev/encoding/xml
- OPDS spec: https://specs.opds.io/opds-catalog-1-2
- Atom spec: https://tools.ietf.org/html/rfc4287
- Reference: run Calibre Content Server, visit `/opds`, view source XML

### SQLite (Steps 1, 7)
- `database/sql` guide: https://go.dev/doc/database/sql
- `modernc.org/sqlite` docs: https://pkg.go.dev/modernc.org/sqlite
- SQLite WAL mode: https://www.sqlite.org/wal.html

### Docker + k3s (Step 14)
- [Multi-stage Docker builds](https://docs.docker.com/build/building/multistage/)
- Your existing `veridian-apps` chart as a template

---

## Notes and Decisions

### Why not use Calibre's database directly?
Calibre's schema is complex (15+ tables with link tables) and tightly coupled
to its internal state. Using our own schema means:
- Full control over the data model
- No risk of breaking Calibre compatibility with a wrong write
- Simpler code (5 tables vs 15+)
- Import/export to Calibre format as a feature, not a constraint

### Why pure-Go SQLite (modernc.org/sqlite)?
- No CGO dependency — easy cross-compilation
- Static binary works in `FROM scratch` containers
- Slightly slower than CGO sqlite3, but fine for a personal library of ~1000
  books
- No system libraries needed in the container

### Why chi router over stdlib?
- Go 1.22+ has improved `http.ServeMux` with pattern routing, which could work
- chi adds: middleware composition, URL parameters (`/book/{id}`), sub-routers
- It's lightweight, stdlib-compatible, and widely used
- You could swap it out for stdlib later if you want zero deps

### Why not a SPA frontend?
- Server-rendered HTML is simpler to build and deploy
- No build step for frontend assets
- No JS framework to learn alongside Go
- The web UI is for library management — not a daily-driver app
- If you want a fancy frontend later, the JSON API is already there

### EPUB parsing edge cases to watch for
- EPUB 2 vs EPUB 3: metadata location differs slightly
- Multiple creators: use `opf:role` attribute to distinguish author vs
  editor vs translator
- ISBN in `<dc:identifier>`: may have `opf:scheme="ISBN"` attribute, or may
  be a bare `urn:isbn:` URI
- Non-ASCII characters in metadata: ensure UTF-8 handling throughout
- DRM-protected EPUBs: will fail to parse — catch and report gracefully