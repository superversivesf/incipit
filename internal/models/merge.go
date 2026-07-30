package models

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
		book.Series = lookup.Series
		book.Pages = lookup.Pages
		book.Rating = lookup.Rating
		book.Description = lookup.Description
	}

	book.TitleSort = SortTitle(book.Title)
	book.AuthorSort = SortTitle(book.Author)

	return book
}
