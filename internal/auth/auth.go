package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Config holds the configuration for the auth package.
type Config struct {
	SupabaseURL string
	AnonKey     string
	JWTSecret   string
	Port        string
}

// LoginHandler initiates a PKCE OAuth flow with Google SSO via Supabase.
// It generates a code_verifier + code_challenge, stores state in a cookie,
// and redirects to the Supabase authorization URL.
func LoginHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := randomBase64URL(16)
		verifier := randomBase64URL(32)
		challenge := pkceChallenge(verifier)

		payload, err := json.Marshal(map[string]string{"state": state, "verifier": verifier})
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "pkce_state",
			Value:    base64.RawURLEncoding.EncodeToString(payload),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Path:     "/",
		})

		port := cfg.Port
		if port == "" {
			port = "8080"
		}
		redirectURI := fmt.Sprintf("http://localhost:%s/auth/callback", port)
		authURL := fmt.Sprintf(
			"%s/auth/v1/authorize?provider=google&response_type=code&redirect_to=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
			cfg.SupabaseURL,
			url.QueryEscape(redirectURI),
			url.QueryEscape(challenge),
			url.QueryEscape(state),
		)

		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

// CallbackHandler validates the CSRF state, exchanges the code for a Supabase JWT,
// and returns it as JSON: { "token": "<jwt>" }.
func CallbackHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stateParam := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")

		cookie, err := r.Cookie("pkce_state")
		if err != nil {
			http.Error(w, "missing state cookie", http.StatusBadRequest)
			return
		}

		raw, err := base64.RawURLEncoding.DecodeString(cookie.Value)
		if err != nil {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		var cookiePayload struct {
			State    string `json:"state"`
			Verifier string `json:"verifier"`
		}
		if err := json.Unmarshal(raw, &cookiePayload); err != nil || cookiePayload.State != stateParam {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}

		tokenURL := cfg.SupabaseURL + "/auth/v1/token?grant_type=pkce"
		body := fmt.Sprintf(`{"auth_code":%q,"code_verifier":%q}`, code, cookiePayload.Verifier)
		resp, err := http.Post(tokenURL, "application/json", strings.NewReader(body)) //nolint:noctx
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		respBody, _ := io.ReadAll(resp.Body)
		var tokenResp struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(respBody, &tokenResp); err != nil || tokenResp.AccessToken == "" {
			http.Error(w, "invalid token response", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": tokenResp.AccessToken})
	}
}

// NewUserClient returns an *http.Client that injects Authorization: Bearer <jwt>
// on every outbound request (RFC §4.10: never use service_role in user-facing paths).
func NewUserClient(jwt string) *http.Client {
	return &http.Client{
		Transport: &bearerTransport{base: http.DefaultTransport, token: jwt},
	}
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func randomBase64URL(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
