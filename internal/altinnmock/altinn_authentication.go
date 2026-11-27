package altinnmock

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	altinnTokenLifetime = 30 * time.Minute
	bearerScheme        = "Bearer "
)

// Altinn drops these claims of the Maskinporten token, and sets its own iss and sid
var droppedClaims = []string{"aud", "iss", "client_amr", "sid"}

// Plays Altinn's exchange of a Maskinporten token for an Altinn token. Like Altinn, the Altinn token has the
// Maskinporten token's claims, scope and consumer among them, and Altinn's own on top
func handleExchange(tokens *tokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := maskinportenClaims(bearerToken(r))
		if !ok {
			writeProblemJSON(w, http.StatusUnauthorized, "https://tools.ietf.org/html/rfc9110#section-15.5.2",
				"Unauthorized")
			return
		}
		now := time.Now()
		expiresAt := now.Add(altinnTokenLifetime)
		consumer, _ := claims["consumer"].(map[string]any)
		consumerID, _ := consumer["ID"].(string)
		_, organizationNumber, _ := strings.Cut(consumerID, ":")
		altinnClaims := map[string]any{}
		for key, value := range claims {
			altinnClaims[key] = value
		}
		for _, dropped := range droppedClaims {
			delete(altinnClaims, dropped)
		}
		altinnClaims["urn:altinn:orgNumber"] = organizationNumber
		altinnClaims["urn:altinn:authenticatemethod"] = "maskinporten"
		altinnClaims["urn:altinn:authlevel"] = 3
		altinnClaims["iss"] = baseURLOf(r) + "/authentication/api/v1/openid/"
		altinnClaims["sid"] = uuid.NewString()
		altinnClaims["iat"] = now.Unix()
		altinnClaims["nbf"] = now.Unix()
		altinnClaims["exp"] = expiresAt.Unix()
		altinnToken := tokens.issueAltinn(altinnClaims, expiresAt)

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(altinnToken))
	}
}

// Altinnator gets its Maskinporten token from Texas, which gets it from the mock's Maskinporten without a signature. So
// unlike Altinn, the mock does not check the token's signature, only that it is a JWT that has not expired
func maskinportenClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	claims, err := decodeJSON(parts[1])
	if err != nil {
		return nil, false
	}
	expiresAt, ok := epochSeconds(claims["exp"])
	if !ok || expiresAt.Before(time.Now()) {
		return nil, false
	}
	return claims, true
}

func baseURLOf(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if len(header) < len(bearerScheme) || !strings.EqualFold(header[:len(bearerScheme)], bearerScheme) {
		return ""
	}
	return strings.TrimSpace(header[len(bearerScheme):])
}

func decodeJSON(part string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(part, "="))
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var claims map[string]any
	if err := decoder.Decode(&claims); err != nil || claims == nil {
		return nil, errors.New("not a JSON object")
	}
	return claims, nil
}

func epochSeconds(value any) (time.Time, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return time.Time{}, false
	}
	seconds, err := number.Int64()
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}
