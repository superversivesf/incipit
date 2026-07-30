package lookup

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

type Client interface {
	LookupByISBN(ctx context.Context, isbn string) (*models.LookupResult, error)
	LookupByTitle(ctx context.Context, title, author string) (*models.LookupResult, error)
}
