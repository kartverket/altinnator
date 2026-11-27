package auth

import (
	"regexp"
	"time"

	"github.com/go-logr/logr"
)

// RFC 6750's b64token: anything else could break the Authorization header.
var bearerToken = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

type Token struct {
	value     string
	expiresAt time.Time
}

var _ logr.Marshaler = Token{}

// NewToken lets code outside this package make a Token, e.g. a fake Provider in tests.
func NewToken(value string, expiresAt time.Time) Token {
	return Token{value: value, expiresAt: expiresAt}
}

func (t Token) Reveal() string { return t.value }

func (t Token) String() string {
	return "Token(***, expires " + t.expiresAt.UTC().Format(time.RFC3339) + ")"
}

func (t Token) GoString() string { return t.String() }

func (t Token) MarshalLog() any { return t.String() }

func (t Token) expired(now time.Time, leeway time.Duration) bool {
	return !now.Add(leeway).Before(t.expiresAt)
}
