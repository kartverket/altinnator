package altinnmock

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"
)

type issuedToken struct {
	claims    map[string]any
	expiresAt time.Time
}

// The Altinn tokens the mock has issued, and their claims. They are JWTs without a signature: instead of checking a
// signature, the mock only accepts a token it issued itself, until it expires
type tokens struct {
	mu     sync.Mutex
	altinn map[string]issuedToken
}

func newTokens() *tokens {
	return &tokens{altinn: map[string]issuedToken{}}
}

func (t *tokens) issueAltinn(claims map[string]any, expiresAt time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for token, existing := range t.altinn {
		if existing.expiresAt.Before(time.Now()) {
			delete(t.altinn, token)
		}
	}
	token := unsignedJWT(claims)
	t.altinn[token] = issuedToken{claims, expiresAt}
	return token
}

func unsignedJWT(claims map[string]any) string {
	return base64URL(map[string]any{"alg": "none", "typ": "JWT"}) + "." + base64URL(claims) + "."
}

func base64URL(value map[string]any) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}
