package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// The lifetimes the mock in altinn-management gives its tokens
	maskinportenLifetime = 120 * time.Second
	altinnLifetime       = 30 * time.Minute
)

// fakeClock is the time the Provider and fakeAltinn share. A test moves it forward to make tokens expire.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

type cannedResponse struct {
	status int
	body   string
}

// texasRequest is a token request to Texas, as the fake received it.
type texasRequest struct {
	contentType string
	body        map[string]any
}

// fakeAltinn plays Texas's token endpoint and Altinn's token exchange, and records the requests to them. Its Texas
// gives a new Maskinporten token per request. Like the mock in altinn-management, its Altinn tokens are JWTs without a
// signature. A canned response replaces a token.
type fakeAltinn struct {
	server *httptest.Server
	clock  *fakeClock

	mu                     sync.Mutex
	texasRequests          []texasRequest
	exchangeAuthorizations []string
	altinnTokens           []string
	texasAnswer            *cannedResponse
	exchangeAnswer         *cannedResponse
	exchangeHangs          atomic.Bool
}

func newFakeAltinn(clock *fakeClock) *fakeAltinn {
	fake := &fakeAltinn{clock: clock}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/token", fake.serveTexas)
	mux.HandleFunc("GET "+exchangePath, fake.serveExchange)
	fake.server = httptest.NewServer(mux)
	return fake
}

func (f *fakeAltinn) serveTexas(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.texasRequests = append(f.texasRequests, texasRequest{contentType: r.Header.Get("Content-Type"), body: body})
	if f.texasAnswer != nil {
		w.WriteHeader(f.texasAnswer.status)
		_, _ = w.Write([]byte(f.texasAnswer.body))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": fmt.Sprintf("maskinporten-token-%d", len(f.texasRequests)),
		"token_type":   "Bearer",
		"expires_in":   int64(maskinportenLifetime / time.Second),
	})
}

func (f *fakeAltinn) serveExchange(w http.ResponseWriter, r *http.Request) {
	if f.exchangeHangs.Load() {
		<-r.Context().Done()
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.exchangeAuthorizations = append(f.exchangeAuthorizations, r.Header.Get("Authorization"))
	if f.exchangeAnswer != nil {
		w.WriteHeader(f.exchangeAnswer.status)
		_, _ = w.Write([]byte(f.exchangeAnswer.body))
		return
	}
	token := unsignedJWT(fmt.Sprintf(`{"jti":"altinn-token-%d","exp":%d}`,
		len(f.exchangeAuthorizations), f.clock.Now().Add(altinnLifetime).Unix()))
	f.altinnTokens = append(f.altinnTokens, token)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(token))
}

func (f *fakeAltinn) texas() []texasRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.texasRequests)
}

// skippedCache tells, per request to Texas, whether it asked Texas to skip its cache.
func (f *fakeAltinn) skippedCache() []bool {
	requests := f.texas()
	skipped := make([]bool, 0, len(requests))
	for _, request := range requests {
		skipped = append(skipped, request.body["skip_cache"] == true)
	}
	return skipped
}

func (f *fakeAltinn) exchanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.exchangeAuthorizations)
}

func (f *fakeAltinn) issuedAltinnTokens() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.altinnTokens)
}

// answer makes Texas and the exchange answer with the canned responses. nil gives a token again.
func (f *fakeAltinn) answer(texas, exchange *cannedResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.texasAnswer = texas
	f.exchangeAnswer = exchange
}

// unsignedJWT makes a JWT without a signature, like the mock's tokens.
func unsignedJWT(payload string) string {
	encode := base64.RawURLEncoding.EncodeToString
	return encode([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + encode([]byte(payload)) + "."
}

var _ = Describe("Provider", func() {
	var (
		clock    *fakeClock
		fake     *fakeAltinn
		provider *Provider
	)

	BeforeEach(func() {
		clock = &fakeClock{now: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)}
		fake = newFakeAltinn(clock)
		DeferCleanup(fake.server.Close)

		provider = newProvider(Config{
			TexasTokenURL:   fake.server.URL + "/api/v1/token",
			PlatformBaseURL: fake.server.URL,
		}, fake.server.Client(), clock.Now)
	})

	Describe("getting a token", func() {
		It("trades a Maskinporten token for an Altinn token", func(ctx SpecContext) {
			token, err := provider.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(fake.exchanges()).To(Equal([]string{"Bearer maskinporten-token-1"}))
			Expect(token.Reveal()).To(Equal(fake.issuedAltinnTokens()[0]))
			Expect(token.expiresAt).To(BeTemporally("==", clock.Now().Add(altinnLifetime)))
		})

		It("asks Texas for a Maskinporten token with the resource write scope", func(ctx SpecContext) {
			_, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.texas()).To(Equal([]texasRequest{{
				contentType: "application/json",
				body: map[string]any{
					"identity_provider": "maskinporten",
					"target":            "altinn:resourceregistry/resource.write",
				},
			}}))
		})
	})

	Describe("caching", func() {
		It("reuses the token until 30 seconds before it expires", func(ctx SpecContext) {
			first, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())

			clock.Advance(altinnLifetime - refreshLeeway - time.Second)
			again, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(again).To(Equal(first))
			Expect(fake.texas()).To(HaveLen(1))
			Expect(fake.exchanges()).To(HaveLen(1))

			clock.Advance(time.Second)
			renewed, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(renewed.Reveal()).NotTo(Equal(first.Reveal()))
			Expect(renewed.expiresAt).To(BeTemporally("==", clock.Now().Add(altinnLifetime)))
			// Texas decides whether the Maskinporten token can be reused, so the renewal lets Texas use its cache
			Expect(fake.skippedCache()).To(Equal([]bool{false, false}))
			Expect(fake.exchanges()).To(Equal([]string{"Bearer maskinporten-token-1", "Bearer maskinporten-token-2"}))
		})

		It("gets one token for callers at the same time", func(ctx SpecContext) {
			tokens := make([]Token, 10)
			var callers sync.WaitGroup
			for i := range tokens {
				callers.Go(func() {
					defer GinkgoRecover()
					var err error
					tokens[i], err = provider.Token(ctx)
					Expect(err).NotTo(HaveOccurred())
				})
			}
			callers.Wait()

			Expect(tokens).To(HaveEach(Equal(tokens[0])))
			Expect(fake.texas()).To(HaveLen(1))
			Expect(fake.exchanges()).To(HaveLen(1))
		})

		It("gives up on a fetch that hangs, so callers waiting for it get through", SpecTimeout(5*time.Second),
			func(ctx SpecContext) {
				provider.altinnTokens.timeout = 100 * time.Millisecond
				fake.exchangeHangs.Store(true)

				errs := make([]error, 2)
				var callers sync.WaitGroup
				for i := range errs {
					callers.Go(func() { _, errs[i] = provider.Token(ctx) })
				}
				callers.Wait()

				Expect(errs).To(HaveEach(MatchError(ContainSubstring("context deadline exceeded"))))
			})
	})

	Describe("when Altinn answers 401", func() {
		It("gets a new Altinn token, and lets Texas use its cached Maskinporten token", func(ctx SpecContext) {
			refused, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())

			provider.Refused(refused)
			renewed, err := provider.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(renewed.Reveal()).NotTo(Equal(refused.Reveal()))
			Expect(fake.skippedCache()).To(Equal([]bool{false, false}))
			Expect(fake.exchanges()).To(HaveLen(2))
		})

		It("keeps a newer token when an older one is refused late", func(ctx SpecContext) {
			refused, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())
			provider.Refused(refused)
			renewed, err := provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())

			provider.Refused(refused)
			again, err := provider.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(again).To(Equal(renewed))
			Expect(fake.exchanges()).To(HaveLen(2))
		})
	})

	Describe("when the Altinn token exchange fails", func() {
		It("asks Texas for a new Maskinporten token for the next try, and only that one", func(ctx SpecContext) {
			fake.answer(nil, &cannedResponse{http.StatusUnauthorized, `{"title":"Unauthorized","status":401}`})
			_, err := provider.Token(ctx)
			Expect(err).To(HaveOccurred())

			fake.answer(nil, nil)
			_, err = provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())
			clock.Advance(altinnLifetime)
			_, err = provider.Token(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(fake.skippedCache()).To(Equal([]bool{false, true, false}))
			Expect(fake.exchanges()).To(Equal([]string{
				"Bearer maskinporten-token-1", "Bearer maskinporten-token-2", "Bearer maskinporten-token-3"}))
		})

		It("keeps asking Texas for a new Maskinporten token when Texas fails", func(ctx SpecContext) {
			fake.answer(nil, &cannedResponse{http.StatusUnauthorized, `{"title":"Unauthorized","status":401}`})
			_, err := provider.Token(ctx)
			Expect(err).To(HaveOccurred())
			fake.answer(&cannedResponse{http.StatusInternalServerError, ""}, nil)
			_, err = provider.Token(ctx)
			Expect(err).To(HaveOccurred())

			fake.answer(nil, nil)
			_, err = provider.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(fake.skippedCache()).To(Equal([]bool{false, true, true}))
		})
	})

	Describe("errors", func() {
		// The messages are matched in full, so they cannot quote a token
		DescribeTable("reports a failed token request to Texas",
			func(ctx SpecContext, answer cannedResponse, message string) {
				fake.answer(&answer, nil)

				_, err := provider.Token(ctx)

				Expect(err).To(MatchError(message))
				Expect(fake.exchanges()).To(BeEmpty())
			},
			Entry("with the OAuth error from Texas",
				cannedResponse{http.StatusInternalServerError,
					`{"error":"invalid_client","error_description":"Client authentication failed"}`},
				"the token request to Texas failed with HTTP 500: invalid_client: Client authentication failed"),
			Entry("with only the status when the body is not an OAuth error",
				cannedResponse{http.StatusServiceUnavailable, `<html>secret-token</html>`},
				"the token request to Texas failed with HTTP 503"),
			Entry("when the body is not JSON",
				cannedResponse{http.StatusOK, `secret-token`},
				"the response from Texas is not JSON"),
			Entry("when there is no access_token",
				cannedResponse{http.StatusOK, `{"expires_in":120}`},
				"the response from Texas has no access_token"),
			Entry("when the access_token cannot go in an Authorization header",
				cannedResponse{http.StatusOK, `{"access_token":"secret token","expires_in":120}`},
				"the access_token from Texas is not a valid bearer token"),
			Entry("when there is no expires_in",
				cannedResponse{http.StatusOK, `{"access_token":"secret-token"}`},
				"the response from Texas has no expires_in"),
		)

		DescribeTable("reports a failed Altinn token exchange",
			func(ctx SpecContext, answer cannedResponse, message string) {
				fake.answer(nil, &answer)

				_, err := provider.Token(ctx)

				Expect(err).To(MatchError(message))
			},
			Entry("with the traceId from Altinn's problem details",
				cannedResponse{http.StatusUnauthorized, `{"title":"Unauthorized","status":401,"traceId":"00-abc-01"}`},
				"the Altinn token exchange failed with HTTP 401 (traceId 00-abc-01)"),
			Entry("with the detail and traceId from Altinn's problem details",
				cannedResponse{http.StatusForbidden,
					`{"title":"Forbidden","status":403,"detail":"Missing scope","traceId":"00-def-01"}`},
				"the Altinn token exchange failed with HTTP 403: Missing scope (traceId 00-def-01)"),
			Entry("with only the status when the problem details have no detail or traceId",
				cannedResponse{http.StatusUnauthorized, `{"title":"Unauthorized","status":401}`},
				"the Altinn token exchange failed with HTTP 401"),
			Entry("without the body, which can be a token",
				cannedResponse{http.StatusInternalServerError, `secret-token`},
				"the Altinn token exchange failed with HTTP 500"),
			Entry("when the token is empty",
				cannedResponse{http.StatusOK, " \n"},
				"the Altinn token exchange answered with an empty token"),
			Entry("when the token cannot go in an Authorization header",
				cannedResponse{http.StatusOK, `secret token`},
				"the token from the Altinn token exchange is not a valid bearer token"),
			Entry("when the token is not a JWT",
				cannedResponse{http.StatusOK, `secret-token`},
				"the Altinn token is not a JWT"),
			Entry("when the payload is not base64url",
				cannedResponse{http.StatusOK, `a.b.c`},
				"the Altinn token's payload is not base64url"),
			Entry("when the payload is not JSON",
				cannedResponse{http.StatusOK, unsignedJWT(`secret`)},
				"the Altinn token's payload is not JSON with a numeric exp"),
			Entry("when exp is not a number",
				cannedResponse{http.StatusOK, unsignedJWT(`{"exp":"soon"}`)},
				"the Altinn token's payload is not JSON with a numeric exp"),
			Entry("when there is no exp",
				cannedResponse{http.StatusOK, unsignedJWT(`{"jti":"secret"}`)},
				"the Altinn token has no exp claim"),
		)

		It("reports that Texas cannot be reached", func(ctx SpecContext) {
			fake.server.Close()

			_, err := provider.Token(ctx)

			Expect(err).To(MatchError(ContainSubstring("calling Texas")))
		})

		It("does not keep a failure, so the next call tries again", func(ctx SpecContext) {
			fake.answer(&cannedResponse{http.StatusBadGateway, ""}, nil)
			_, err := provider.Token(ctx)
			Expect(err).To(HaveOccurred())

			fake.answer(nil, nil)
			token, err := provider.Token(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(token.Reveal()).To(Equal(fake.issuedAltinnTokens()[0]))
			Expect(fake.texas()).To(HaveLen(2))
			Expect(fake.exchanges()).To(HaveLen(1))
		})
	})
})
