package auth

import (
	"context"
	"sync"
	"time"
)

type tokenCache struct {
	mu      sync.Mutex
	token   *Token
	now     func() time.Time
	leeway  time.Duration
	timeout time.Duration
}

func (c *tokenCache) get(ctx context.Context, fetch func(context.Context) (Token, error)) (Token, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != nil && !c.token.expired(c.now(), c.leeway) {
		return *c.token, nil
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	token, err := fetch(ctx)
	if err != nil {
		return Token{}, err
	}
	c.token = &token
	return token, nil
}

func (c *tokenCache) clear(value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != nil && c.token.value == value {
		c.token = nil
	}
}
