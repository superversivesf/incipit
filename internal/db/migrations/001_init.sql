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
    document_hash TEXT,
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