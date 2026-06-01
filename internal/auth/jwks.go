package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// Supabase signs login tokens (Google SSO, password grant) with ASYMMETRIC keys
// — ES256 by default — exposed as a JWKS at
// {SupabaseURL}/auth/v1/.well-known/jwks.json. The legacy HS256 symmetric secret
// still validates (dual). This file resolves the asymmetric public key by `kid`.

var (
	jwksMu      sync.Mutex
	jwksByKID   = map[string]any{}            // kid -> *ecdsa.PublicKey | *rsa.PublicKey
	jwksFetched = map[string]time.Time{}      // supabaseURL -> last fetch
)

// jwksMinRefresh bounds how often we re-fetch the JWKS on an unknown kid, so a
// stream of bogus kids can't hammer the JWKS endpoint.
const jwksMinRefresh = 5 * time.Minute

// publicKeyForKID returns the public key for kid, fetching the JWKS from
// supabaseURL on a cache miss (rate-limited per URL).
func publicKeyForKID(supabaseURL, kid string) (any, error) {
	jwksMu.Lock()
	defer jwksMu.Unlock()

	if k, ok := jwksByKID[kid]; ok {
		return k, nil
	}
	if last, ok := jwksFetched[supabaseURL]; ok && time.Since(last) < jwksMinRefresh {
		return nil, fmt.Errorf("no JWK for kid %q", kid)
	}
	if err := fetchJWKSLocked(supabaseURL); err != nil {
		return nil, err
	}
	if k, ok := jwksByKID[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("no JWK for kid %q", kid)
}

// fetchJWKSLocked fetches + parses the JWKS into jwksByKID. Caller holds jwksMu.
func fetchJWKSLocked(supabaseURL string) error {
	resp, err := http.Get(supabaseURL + "/auth/v1/.well-known/jwks.json") //nolint:noctx
	if err != nil {
		return fmt.Errorf("jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}

	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}

	jwksFetched[supabaseURL] = time.Now()
	for _, k := range doc.Keys {
		if k.Kid == "" {
			continue
		}
		pub, err := k.publicKey()
		if err != nil {
			continue
		}
		jwksByKID[k.Kid] = pub
	}
	return nil
}

type jwk struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	Kid string `json:"kid"`
	X   string `json:"x"`
	Y   string `json:"y"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "EC":
		if k.Crv != "P-256" {
			return nil, fmt.Errorf("unsupported EC curve %q", k.Crv)
		}
		x, err := b64uBigInt(k.X)
		if err != nil {
			return nil, err
		}
		y, err := b64uBigInt(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
	case "RSA":
		n, err := b64uBigInt(k.N)
		if err != nil {
			return nil, err
		}
		e, err := b64uBigInt(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
	default:
		return nil, fmt.Errorf("unsupported JWK kty %q", k.Kty)
	}
}

func b64uBigInt(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}
