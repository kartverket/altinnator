package altinnmock_test

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A JWT grant, the way Texas makes one: signed with the client's key, which the mock does not check
func grant(change ...func(map[string]any)) string {
	now := time.Now()
	claims := map[string]any{
		"iss":   "dev-client",
		"aud":   server.URL + "/",
		"scope": writeScope,
		"iat":   now.Unix(),
		"nbf":   now.Unix(),
		"exp":   now.Add(30 * time.Second).Unix(),
		"jti":   "a-grant",
	}
	for _, c := range change {
		c(claims)
	}
	return encode(map[string]any{"alg": "RS256", "kid": "local-dev"}) + "." + encode(claims) + ".c2lnbmF0dXJl"
}

func requestToken(form url.Values) response {
	return send(http.MethodPost, "/token", strings.NewReader(form.Encode()), map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
	})
}

func jwtGrant(assertion string) url.Values {
	return url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
}

var _ = Describe("Maskinporten", func() {
	It("gives Texas a token with the scope it asks for, on behalf of Kartverket", func() {
		answer := requestToken(jwtGrant(grant()))

		Expect(answer.status).To(Equal(http.StatusOK), answer.body)
		Expect(answer.mediaType()).To(Equal("application/json"))
		Expect(answer.json()).To(HaveKeyWithValue("token_type", "Bearer"))
		Expect(answer.json()).To(HaveKeyWithValue("expires_in", BeNumerically("==", 120)))
		Expect(answer.json()).To(HaveKeyWithValue("scope", writeScope))
		claims := claimsOf(answer.json()["access_token"].(string))
		Expect(claims).To(HaveKeyWithValue("iss", server.URL+"/"))
		Expect(claims).To(HaveKeyWithValue("scope", writeScope))
		Expect(claims).To(HaveKeyWithValue("client_id", "dev-client"))
		Expect(claims).To(HaveKeyWithValue("consumer", HaveKeyWithValue("ID", "0192:"+kartverket)))
		Expect(claims).To(HaveKeyWithValue("consumer_prefix", "kartverk"))
		Expect(claims["exp"].(float64) - float64(time.Now().Unix())).To(BeNumerically("~", 120, 10))
	})

	It("gives a token that Altinn's exchange accepts", func() {
		token := requestToken(jwtGrant(grant())).json()["access_token"].(string)

		answer := exchange(map[string]string{"Authorization": "Bearer " + token})

		Expect(answer.status).To(Equal(http.StatusOK), answer.body)
		Expect(claimsOf(answer.body)).To(HaveKeyWithValue("urn:altinn:orgNumber", kartverket))
	})

	DescribeTable("answers invalid_grant to a request it cannot give a token for",
		func(form func() url.Values) {
			answer := requestToken(form())

			Expect(answer.status).To(Equal(http.StatusBadRequest))
			Expect(answer.json()).To(HaveKeyWithValue("error", "invalid_grant"))
		},
		Entry("another grant type", func() url.Values {
			return url.Values{"grant_type": {"client_credentials"}, "assertion": {grant()}}
		}),
		Entry("no assertion", func() url.Values {
			return url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}}
		}),
		Entry("an assertion that is not a JWT", func() url.Values { return jwtGrant("not-a-jwt") }),
		Entry("a grant without scope", func() url.Values {
			return jwtGrant(grant(func(c map[string]any) { delete(c, "scope") }))
		}),
	)

	It("tells Texas where its token endpoint and keys are", func() {
		answer := get("/.well-known/oauth-authorization-server")

		Expect(answer.status).To(Equal(http.StatusOK))
		Expect(answer.json()).To(HaveKeyWithValue("issuer", server.URL+"/"))
		Expect(answer.json()).To(HaveKeyWithValue("token_endpoint", server.URL+"/token"))
		Expect(answer.json()).To(HaveKeyWithValue("jwks_uri", server.URL+"/jwk"))
	})

	It("gives Texas a key set when it starts", func() {
		answer := get("/jwk")

		Expect(answer.status).To(Equal(http.StatusOK))
		Expect(answer.json()).To(HaveKeyWithValue("keys", BeEmpty()))
	})
})
