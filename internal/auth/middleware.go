package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AuthMiddleware validates the Bearer JWT from the Authorization header.
// On success: places user_id and raw JWT in the request context, calls next.
// On failure: responds 401 with the RFC §4.11 error envelope.
func AuthMiddleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearer(r)
			if token == "" {
				writeAuthError(w, "missing or malformed Authorization header")
				return
			}

			userID, err := validateJWT(token, cfg.JWTSecret, cfg.SupabaseURL)
			if err != nil {
				writeAuthError(w, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			ctx = ContextWithJWT(ctx, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

// validateJWT verifies a Supabase token. It accepts both the legacy HS256
// symmetric secret AND asymmetric ES256/RS256 tokens (the modern Supabase
// default), resolving the public key by `kid` from the project JWKS.
func validateJWT(tokenString, secret, supabaseURL string) (uuid.UUID, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&jwt.RegisteredClaims{},
		func(t *jwt.Token) (any, error) {
			switch t.Method.(type) {
			case *jwt.SigningMethodHMAC:
				if secret == "" {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			case *jwt.SigningMethodECDSA, *jwt.SigningMethodRSA:
				kid, _ := t.Header["kid"].(string)
				if kid == "" || supabaseURL == "" {
					return nil, jwt.ErrSignatureInvalid
				}
				return publicKeyForKID(supabaseURL, kid)
			default:
				return nil, jwt.ErrSignatureInvalid
			}
		},
		jwt.WithValidMethods([]string{"HS256", "ES256", "RS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return uuid.Nil, err
	}
	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code":       "unauthenticated",
			"message":    message,
			"request_id": uuid.New().String(),
		},
	})
}
