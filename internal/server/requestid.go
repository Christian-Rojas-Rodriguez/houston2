package server

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

type requestIDKey struct{}

// RequestIDMiddleware generates a UUID per request, stores it in context,
// and sets X-Request-ID on the response before calling next.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext retrieves the request ID stored by RequestIDMiddleware.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
