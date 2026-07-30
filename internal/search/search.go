package search

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

type Opts struct {
	Limit  int
	Offset int
}

type Searcher interface {
	Search(ctx context.Context, q string, opts Opts) ([]models.Book, int, error)
}
