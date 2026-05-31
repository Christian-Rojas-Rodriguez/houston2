package middleware

import "net/http"

// RequireRole returns middleware that rejects requests where the caller's role
// is below minRole. Must be chained after TenantMiddleware.
// Returns 500 if TenantContext is absent — signals a middleware ordering bug.
func RequireRole(minRole Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tc, ok := TenantFromContext(r.Context())
			if !ok {
				writeErrorJSON(w, http.StatusInternalServerError, "internal_error", "TenantContext missing — check middleware order")
				return
			}
			if tc.Role < minRole {
				writeErrorJSON(w, http.StatusForbidden, "forbidden", "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
