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
