package middleware

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// TenantQuerier is the port this package owns (RFC §4.14: interfaces defined
// in the consumer). The concrete Supabase PostgREST adapter lives in internal/server.
type TenantQuerier interface {
	CurrentTenant(ctx context.Context, userID uuid.UUID) (TenantContext, error)
}

// ErrNoMembership is returned by TenantQuerier when the user has no row in memberships.
var ErrNoMembership = errors.New("no membership")
