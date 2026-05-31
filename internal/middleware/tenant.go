package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
)

// TenantContext holds the tenant context derived from the memberships table.
type TenantContext struct {
	OrgID   uuid.UUID
	GroupID uuid.UUID
	Role    Role
}

type tenantCtxKey struct{}

// TenantMiddleware derives tenant context for each request and stores it in ctx.
// Requires auth.UserIDFromContext to return a valid UUID (AuthMiddleware must run first).
// Returns 401 if no user_id in context; 403 if no membership; 500 on other errors.
func TenantMiddleware(q TenantQuerier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				writeErrorJSON(w, http.StatusUnauthorized, "unauthenticated", "authentication required")
				return
			}

			tc, err := q.CurrentTenant(r.Context(), userID)
			if err != nil {
				if errors.Is(err, ErrNoMembership) {
					writeErrorJSON(w, http.StatusForbidden, "forbidden", "no membership for this user")
					return
				}
				writeErrorJSON(w, http.StatusInternalServerError, "internal_error", "failed to resolve tenant")
				return
			}

			ctx := context.WithValue(r.Context(), tenantCtxKey{}, tc)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFromContext retrieves the TenantContext stored by TenantMiddleware.
func TenantFromContext(ctx context.Context) (TenantContext, bool) {
	tc, ok := ctx.Value(tenantCtxKey{}).(TenantContext)
	return tc, ok
}
