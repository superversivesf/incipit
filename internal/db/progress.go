package db

import (
	"database/sql"
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) GetProgress(documentHash string, userID int64) (*models.ReadingProgress, error) {
	var p models.ReadingProgress
	var bookID *int64
	err := d.db.QueryRow(
		`SELECT book_id, document_hash, user_id, percentage, progress, device, updated
		 FROM reading_progress WHERE document_hash = ? AND user_id = ?`,
		documentHash, userID,
	).Scan(&bookID, &p.DocumentHash, &p.UserID, &p.Percentage, &p.Progress, &p.Device, &p.Updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no progress for hash=%s user=%d: %w", documentHash, userID, err)
		}
		return nil, fmt.Errorf("getting progress: %w", err)
	}
	p.BookID = bookID
	return &p, nil
}

func (d *DB) UpsertProgress(p *models.ReadingProgress) error {
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

type BookProgress struct {
	BookID     int64
	Title      string
	Author     string
	CoverPath  string
	Percentage float64
	Device     string
	Updated    string
}

func (d *DB) ListReadingProgress(userID int64) ([]BookProgress, error) {
	rows, err := d.db.Query(
		`SELECT b.id, b.title, b.author, b.cover_path,
		   rp.percentage, rp.device, rp.updated
		 FROM reading_progress rp
		 JOIN books b ON b.id = rp.book_id
		 WHERE rp.user_id = ? AND rp.book_id IS NOT NULL
		 ORDER BY rp.updated DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing reading progress: %w", err)
	}
	defer rows.Close()

	var results []BookProgress
	for rows.Next() {
		var bp BookProgress
		if err := rows.Scan(&bp.BookID, &bp.Title, &bp.Author, &bp.CoverPath,
			&bp.Percentage, &bp.Device, &bp.Updated); err != nil {
			return nil, fmt.Errorf("scanning progress row: %w", err)
		}
		results = append(results, bp)
	}
	return results, rows.Err()
}
