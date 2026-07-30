package db

import (
	"fmt"

	"github.com/jason/incipit/internal/models"
)

func (d *DB) DistinctAuthors() ([]string, error) {
	rows, err := d.db.Query(`SELECT DISTINCT author FROM books ORDER BY author_sort`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var authors []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		authors = append(authors, a)
	}
	return authors, rows.Err()
}

func (d *DB) CountBooksByAuthor(author string) int {
	var c int
	d.db.QueryRow("SELECT COUNT(*) FROM books WHERE author = ?", author).Scan(&c)
	return c
}

func (d *DB) CountBooksBySeries(series string) int {
	var c int
	d.db.QueryRow("SELECT COUNT(*) FROM books WHERE series = ?", series).Scan(&c)
	return c
}

func (d *DB) CountBooksByTag(tagID int64) int {
	var c int
	d.db.QueryRow("SELECT COUNT(*) FROM book_tags WHERE tag_id = ?", tagID).Scan(&c)
	return c
}

func (d *DB) CountedBooksBy(column, value string, limit, offset int) ([]models.Book, int) {
	var total int
	d.db.QueryRow("SELECT COUNT(*) FROM books WHERE "+column+" = ?", value).Scan(&total)
	books, _ := d.queryBooks(
		"SELECT id, title, title_sort, author, author_sort, series, series_index, isbn, description, publisher, published, pages, rating, cover_path, file_path, file_hash, file_size, added, updated FROM books WHERE "+column+" = ? ORDER BY title_sort LIMIT ? OFFSET ?",
		value, limit, offset)
	return books, total
}

func (d *DB) ListTags() ([]models.Tag, error) {
	rows, err := d.db.Query(`SELECT id, name, parent_id FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.ParentID); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (d *DB) CreateTag(name string, parentID *int64) (int64, error) {
	result, err := d.db.Exec(`INSERT INTO tags (name, parent_id) VALUES (?, ?)`, name, parentID)
	if err != nil {
		return 0, fmt.Errorf("creating tag: %w", err)
	}
	return result.LastInsertId()
}

func (d *DB) UpdateTag(id int64, name string, parentID *int64) error {
	_, err := d.db.Exec(`UPDATE tags SET name=?, parent_id=? WHERE id=?`, name, parentID, id)
	return err
}

func (d *DB) DeleteTag(id int64) error {
	_, err := d.db.Exec("DELETE FROM tags WHERE id = ?", id)
	return err
}

func (d *DB) GetTagsForBook(bookID int64) ([]models.Tag, error) {
	rows, err := d.db.Query(
		`SELECT t.id, t.name, t.parent_id FROM tags t
		 JOIN book_tags bt ON bt.tag_id = t.id WHERE bt.book_id = ? ORDER BY t.name`, bookID)
	if err != nil {
		return nil, fmt.Errorf("getting tags for book: %w", err)
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.ParentID); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (d *DB) AddTagToBook(bookID, tagID int64) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO book_tags (book_id, tag_id) VALUES (?, ?)`, bookID, tagID)
	return err
}

func (d *DB) RemoveTagFromBook(bookID, tagID int64) error {
	_, err := d.db.Exec(`DELETE FROM book_tags WHERE book_id=? AND tag_id=?`, bookID, tagID)
	return err
}

type SeriesInfo struct {
	Name      string
	BookCount int
}

func (d *DB) ListSeries() ([]SeriesInfo, error) {
	rows, err := d.db.Query(
		`SELECT series, COUNT(*) FROM books WHERE series IS NOT NULL AND series != '' GROUP BY series ORDER BY series`)
	if err != nil {
		return nil, fmt.Errorf("listing series: %w", err)
	}
	defer rows.Close()

	var series []SeriesInfo
	for rows.Next() {
		var s SeriesInfo
		if err := rows.Scan(&s.Name, &s.BookCount); err != nil {
			return nil, err
		}
		series = append(series, s)
	}
	return series, rows.Err()
}

func (d *DB) RenameSeries(oldName, newName string) error {
	_, err := d.db.Exec("UPDATE books SET series=?, updated=datetime('now') WHERE series=?", newName, oldName)
	return err
}

func (d *DB) BooksBySeries(seriesName string, limit, offset int) ([]models.Book, error) {
	return d.queryBooks("SELECT * FROM books WHERE series = ? ORDER BY series_index LIMIT ? OFFSET ?", seriesName, limit, offset)
}

func (d *DB) BooksByAuthor(author string, limit, offset int) ([]models.Book, error) {
	return d.queryBooks("SELECT * FROM books WHERE author = ? ORDER BY title_sort LIMIT ? OFFSET ?", author, limit, offset)
}

func (d *DB) BooksByTag(tagID int64, limit, offset int) ([]models.Book, error) {
	rows, err := d.db.Query(
		`SELECT b.id, b.title, b.title_sort, b.author, b.author_sort, b.series, b.series_index,
		   b.isbn, b.description, b.publisher, b.published, b.pages, b.rating, b.cover_path,
		   b.file_path, b.file_hash, b.file_size, b.added, b.updated
		 FROM books b JOIN book_tags bt ON bt.book_id = b.id WHERE bt.tag_id = ?
		 ORDER BY b.title_sort LIMIT ? OFFSET ?`, tagID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBooks(rows)
}

func (d *DB) queryBooks(query string, args ...interface{}) ([]models.Book, error) {
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBooks(rows)
}

func scanBooks(rows interface {
	Next() bool
	Scan(...interface{}) error
	Err() error
}) ([]models.Book, error) {
	var books []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.ID, &b.Title, &b.TitleSort, &b.Author, &b.AuthorSort,
			&b.Series, &b.SeriesIndex, &b.ISBN, &b.Description, &b.Publisher,
			&b.Published, &b.Pages, &b.Rating, &b.CoverPath, &b.FilePath,
			&b.FileHash, &b.FileSize, &b.Added, &b.Updated); err != nil {
			return nil, err
		}
		books = append(books, b)
	}
	return books, rows.Err()
}
