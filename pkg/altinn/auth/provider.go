package auth

import (
	"context"
	"net/http"
	"time"
)

const (
	refreshLeeway = 30 * time.Second
	fetchTimeout  = 10 * time.Second
)

type Config struct {
	// TexasTokenURL is Texas's token endpoint, normally TexasTokenURL.
	TexasTokenURL   string
	PlatformBaseURL string
}

// Provider gives Altinn tokens. It gets a Maskinporten token from Texas, trades it for an Altinn token, and caches the
// Altinn token. Texas caches the Maskinporten token.
type Provider struct {
	texas        *texasClient
	exchanger    *altinnExchanger
	altinnTokens *tokenCache
	// renewMaskinporten makes the next fetch skip Texas's cache. Only the fetch in Token uses it, and altinnTokens runs
	// one fetch at a time.
	renewMaskinporten bool
}

func NewProvider(config Config, httpClient *http.Client) *Provider {
	return newProvider(config, httpClient, time.Now)
}

func newProvider(config Config, httpClient *http.Client, now func() time.Time) *Provider {
	return &Provider{
		texas:        &texasClient{tokenURL: config.TexasTokenURL, httpClient: httpClient, now: now},
		exchanger:    newAltinnExchanger(config.PlatformBaseURL, httpClient),
		altinnTokens: &tokenCache{now: now, leeway: refreshLeeway, timeout: fetchTimeout},
	}
}

func (p *Provider) Token(ctx context.Context) (Token, error) {
	return p.altinnTokens.get(ctx, func(ctx context.Context) (Token, error) {
		maskinportenToken, err := p.texas.fetch(ctx, p.renewMaskinporten)
		if err != nil {
			return Token{}, err
		}
		token, err := p.exchanger.exchange(ctx, maskinportenToken)
		// The Maskinporten token can be why the exchange failed, so the next try gets a new one
		p.renewMaskinporten = err != nil
		if err != nil {
			return Token{}, err
		}
		return token, nil
	})
}

func (p *Provider) Refused(token Token) {
	p.altinnTokens.clear(token.value)
}
