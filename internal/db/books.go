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
