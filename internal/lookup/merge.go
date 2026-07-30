package lookup

import "github.com/jason/incipit/internal/models"

func Merge(ol, gb *models.LookupResult) *models.LookupResult {
	if ol == nil && gb == nil {
		return nil
	}
	if ol == nil {
		return gb
	}
	if gb == nil {
		return ol
	}

	merged := &models.LookupResult{}

	merged.Series = ol.Series
	merged.Subjects = ol.Subjects
	merged.CoverURL = ol.CoverURL

	merged.Rating = gb.Rating
	merged.Description = gb.Description
	merged.Published = gb.Published

	merged.Title = firstNonEmpty(ol.Title, gb.Title)
	merged.Author = firstNonEmpty(ol.Author, gb.Author)
	merged.Pages = firstNonZero(ol.Pages, gb.Pages)
	merged.Publisher = firstNonEmpty(ol.Publisher, gb.Publisher)

	merged.Sources = append(merged.Sources, ol.Sources...)
	merged.Sources = append(merged.Sources, gb.Sources...)

	return merged
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(vals ...int) int {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}
