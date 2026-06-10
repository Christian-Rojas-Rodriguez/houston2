package auth

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	userIDKey contextKey = "user_id"
	jwtKey    contextKey = "jwt"
)

// UserIDFromContext retrieves the authenticated user's UUID from ctx.
// Returns uuid.Nil, false if not set.
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

// ContextWithJWT stores the raw JWT string in ctx for downstream consumers
// (e.g. supabaseQuerier) to call Supabase with the user's own token.
func ContextWithJWT(ctx context.Context, jwt string) context.Context {
	return context.WithValue(ctx, jwtKey, jwt)
}

// JWTFromContext retrieves the raw JWT string stored by ContextWithJWT.
func JWTFromContext(ctx context.Context) (string, bool) {
	j, ok := ctx.Value(jwtKey).(string)
	return j, ok
}
