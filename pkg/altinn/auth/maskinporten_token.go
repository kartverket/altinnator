package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	ResourceWriteScope = "altinn:resourceregistry/resource.write"

	// TexasTokenURL is where Texas, which runs next to Altinnator in its pod, hands out Maskinporten tokens.
	TexasTokenURL = "http://localhost:3000/api/v1/token"

	maxResponseBytes = 1 << 20
)

// texasClient gets Maskinporten tokens from Texas (https://github.com/nais/texas). Texas holds the Maskinporten client
// and its key, signs the grants, and caches the tokens, so Altinnator never sees the key.
type texasClient struct {
	tokenURL   string
	httpClient *http.Client
	now        func() time.Time
}

type texasTokenRequest struct {
	IdentityProvider string `json:"identity_provider"`
	Target           string `json:"target"`
	SkipCache        bool   `json:"skip_cache,omitempty"`
}

type texasTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   *int64 `json:"expires_in"`
}

type texasErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

// fetch gets a Maskinporten token. skipCache makes Texas get a new token from Maskinporten instead of its cached one.
func (c *texasClient) fetch(ctx context.Context, skipCache bool) (Token, error) {
	requestedAt := c.now()
	requestBody, err := json.Marshal(texasTokenRequest{
		IdentityProvider: "maskinporten",
		Target:           ResourceWriteScope,
		SkipCache:        skipCache,
	})
	if err != nil {
		return Token{}, fmt.Errorf("encoding the token request to Texas: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, bytes.NewReader(requestBody))
	if err != nil {
		return Token{}, fmt.Errorf("creating the token request to Texas: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Token{}, fmt.Errorf("calling Texas: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return Token{}, fmt.Errorf("reading the response from Texas: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Token{}, texasError(response.StatusCode, body)
	}

	var parsed texasTokenResponse
	if json.Unmarshal(body, &parsed) != nil {
		return Token{}, errors.New("the response from Texas is not JSON")
	}
	if parsed.AccessToken == "" {
		return Token{}, errors.New("the response from Texas has no access_token")
	}
	if !bearerToken.MatchString(parsed.AccessToken) {
		return Token{}, errors.New("the access_token from Texas is not a valid bearer token")
	}
	if parsed.ExpiresIn == nil {
		return Token{}, errors.New("the response from Texas has no expires_in")
	}
	expiresAt := requestedAt.Add(time.Duration(*parsed.ExpiresIn) * time.Second)
	return Token{value: parsed.AccessToken, expiresAt: expiresAt}, nil
}

// Texas answers errors, its own and Maskinporten's, as OAuth errors. Never the whole body, which can be a token.
func texasError(status int, body []byte) error {
	var parsed texasErrorResponse
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return fmt.Errorf("the token request to Texas failed with HTTP %d: %s: %s",
			status, parsed.Error, parsed.Description)
	}
	return fmt.Errorf("the token request to Texas failed with HTTP %d", status)
}
