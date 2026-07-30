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
