package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const exchangePath = "/authentication/api/v1/exchange/maskinporten"

type altinnExchanger struct {
	exchangeURL string
	httpClient  *http.Client
}

func newAltinnExchanger(platformBaseURL string, httpClient *http.Client) *altinnExchanger {
	return &altinnExchanger{
		exchangeURL: strings.TrimRight(platformBaseURL, "/") + exchangePath,
		httpClient:  httpClient,
	}
}

func (e *altinnExchanger) exchange(ctx context.Context, maskinportenToken Token) (Token, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.exchangeURL, nil)
	if err != nil {
		return Token{}, fmt.Errorf("creating the Altinn token exchange request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+maskinportenToken.Reveal())

	response, err := e.httpClient.Do(request)
	if err != nil {
		return Token{}, fmt.Errorf("calling the Altinn token exchange: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return Token{}, fmt.Errorf("reading the response from the Altinn token exchange: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Token{}, altinnError(response.StatusCode, body)
	}

	// Altinn answers with the token alone, as plain text
	token := strings.TrimSpace(string(body))
	if token == "" {
		return Token{}, errors.New("the Altinn token exchange answered with an empty token")
	}
	if !bearerToken.MatchString(token) {
		return Token{}, errors.New("the token from the Altinn token exchange is not a valid bearer token")
	}
	expiresAt, err := jwtExpiry(token)
	if err != nil {
		return Token{}, err
	}
	return Token{value: token, expiresAt: expiresAt}, nil
}

// Never the whole body, which can be a token
func altinnError(status int, body []byte) error {
	var problem struct {
		Detail  string `json:"detail"`
		TraceID string `json:"traceId"`
	}
	_ = json.Unmarshal(body, &problem)
	message := fmt.Sprintf("the Altinn token exchange failed with HTTP %d", status)
	if problem.Detail != "" {
		message += ": " + problem.Detail
	}
	if problem.TraceID != "" {
		message += " (traceId " + problem.TraceID + ")"
	}
	return errors.New(message)
}

func jwtExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("the Altinn token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, errors.New("the Altinn token's payload is not base64url")
	}
	var claims struct {
		Exp *int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return time.Time{}, errors.New("the Altinn token's payload is not JSON with a numeric exp")
	}
	if claims.Exp == nil {
		return time.Time{}, errors.New("the Altinn token has no exp claim")
	}
	return time.Unix(*claims.Exp, 0), nil
}
