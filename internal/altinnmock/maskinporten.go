package altinnmock

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	jwtBearerGrantType        = "urn:ietf:params:oauth:grant-type:jwt-bearer"
	maskinportenTokenLifetime = 120 * time.Second
	kartverketScopePrefix     = "kartverk"
)

// Plays Maskinporten's token endpoint for Texas, which asks for a token with a JWT grant. The mock gives any client the
// scope it asks for, on behalf of Kartverket, without checking the grant's signature
func handleMaskinportenToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != jwtBearerGrantType {
		writeInvalidGrant(w, "grant_type must be "+jwtBearerGrantType)
		return
	}
	parts := strings.Split(r.PostForm.Get("assertion"), ".")
	if len(parts) != 3 {
		writeInvalidGrant(w, "assertion must be a JWT")
		return
	}
	grant, err := decodeJSON(parts[1])
	if err != nil {
		writeInvalidGrant(w, "The JWT is not base64url-encoded JSON")
		return
	}
	scope, _ := grant["scope"].(string)
	if strings.TrimSpace(scope) == "" {
		writeInvalidGrant(w, "scope is required")
		return
	}

	now := time.Now()
	token := unsignedJWT(map[string]any{
		"iss":        baseURLOf(r) + "/",
		"client_amr": "private_key_jwt",
		"token_type": "Bearer",
		"scope":      scope,
		"consumer":   map[string]any{"authority": "iso6523-actorid-upis", "ID": "0192:971040238"},
		// Altinn requires every scope of a new or changed resource to start with the prefix of the scopes Kartverket owns
		"consumer_prefix": kartverketScopePrefix,
		"client_id":       grant["iss"],
		"iat":             now.Unix(),
		"exp":             now.Add(maskinportenTokenLifetime).Unix(),
		"jti":             uuid.NewString(),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(maskinportenTokenLifetime.Seconds()),
		"scope":        scope,
	})
}

// Texas finds Maskinporten's token endpoint and keys here when it starts
func handleMaskinportenMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := baseURLOf(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":         baseURL + "/",
		"token_endpoint": baseURL + "/token",
		"jwks_uri":       baseURL + "/jwk",
	})
}

// Texas fetches Maskinporten's keys when it starts. The mock's tokens have no signature, so it has no keys to give
func handleMaskinportenKeys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keys": []any{}})
}

func writeInvalidGrant(w http.ResponseWriter, description string) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_grant", "error_description": description})
}
