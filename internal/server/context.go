package server

import (
	"context"

	"github.com/jason/incipit/internal/models"
)

type contextKey string

const userKey contextKey = "user"

func UserFromContext(ctx context.Context) *models.User {
	v, ok := ctx.Value(userKey).(*models.User)
	if !ok {
		return nil
	}
	return v
}
