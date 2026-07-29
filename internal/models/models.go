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
	BookID       *int64
	DocumentHash string
	UserID       int64
	Percentage   float64
	Progress     string
	Device       string
	Updated      string
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
