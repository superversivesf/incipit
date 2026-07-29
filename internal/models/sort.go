package models

import "strings"

var articles = []string{"The ", "A ", "An "}

func SortTitle(title string) string {
	for _, article := range articles {
		if strings.HasPrefix(title, article) {
			return title[len(article):] + ", " + strings.TrimSpace(article)
		}
	}
	return title
}
