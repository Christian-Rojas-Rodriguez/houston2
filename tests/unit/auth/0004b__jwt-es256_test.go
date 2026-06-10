// Tests for the R-AUTH-ES256 fix: AuthMiddleware must validate asymmetric
// ES256 Supabase tokens (the modern default) against the project JWKS, not only
// legacy HS256. The original middleware accepted HS256 only, so real Supabase
// login tokens (ES256, with a `kid`) were rejected with 401.

package auth_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/nomenclator/houston2/internal/auth"
)

// jwksServer serves a single-key EC P-256 JWKS at the Supabase well-known path.
func jwksServer(t *testing.T, pub *ecdsa.PublicKey, kid string) *httptest.Server {
	t.Helper()
	pad := func(b []byte) []byte { p := make([]byte, 32); copy(p[32-len(b):], b); return p }
	x := base64.RawURLEncoding.EncodeToString(pad(pub.X.Bytes()))
	y := base64.RawURLEncoding.EncodeToString(pad(pub.Y.Bytes()))
	body := fmt.Sprintf(`{"keys":[{"kty":"EC","crv":"P-256","kid":%q,"x":%q,"y":%q,"alg":"ES256"}]}`, kid, x, y)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func signES256(t *testing.T, priv *ecdsa.PrivateKey, kid, sub string, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(exp),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	})
	tok.Header["kid"] = kid
	s, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign ES256: %v", err)
	}
	return s
}

func TestAuthMiddleware_ES256_JWKS_Valid(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	kid := "kid-" + uuid.NewString()
	jwks := jwksServer(t, &priv.PublicKey, kid)
	defer jwks.Close()

	userID := uuid.New()
	token := signES256(t, priv, kid, userID.String(), time.Now().Add(time.Hour))

	var called bool
	var gotID uuid.UUID
	h := auth.AuthMiddleware(testCfg(jwks.URL))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		gotID, _ = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("ES256 token rejected: code=%d called=%v", rec.Code, called)
	}
	if gotID != userID {
		t.Errorf("user id = %v, want %v", gotID, userID)
	}
}

func TestAuthMiddleware_ES256_UnknownKID_Rejected(t *testing.T) {
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	jwks := jwksServer(t, &priv.PublicKey, "real-"+uuid.NewString())
	defer jwks.Close()

	token := signES256(t, priv, "absent-"+uuid.NewString(), uuid.New().String(), time.Now().Add(time.Hour))
	h := auth.AuthMiddleware(testCfg(jwks.URL))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unknown kid, got %d", rec.Code)
	}
}
