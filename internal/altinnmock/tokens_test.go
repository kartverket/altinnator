package altinnmock_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Altinn's token exchange", func() {
	It("exchanges a Maskinporten token for an Altinn token with its scope, consumer and client", func() {
		token := maskinportenToken()

		answer := exchange(map[string]string{"Authorization": "Bearer " + token})
		maskinporten, altinn := claimsOf(token), claimsOf(answer.body)

		Expect(answer.status).To(Equal(http.StatusOK))
		Expect(answer.mediaType()).To(Equal("text/plain"))
		Expect(altinn["scope"]).To(Equal(maskinporten["scope"]))
		Expect(altinn["consumer"]).To(Equal(maskinporten["consumer"]))
		Expect(altinn["consumer_prefix"]).To(Equal(maskinporten["consumer_prefix"]))
		Expect(altinn["client_id"]).To(Equal(maskinporten["client_id"]))
	})

	It("gives the Altinn token Altinn's own claims, a lifetime of 30 minutes and drops Maskinporten's", func() {
		altinn := claimsOf(altinnToken(writeScope))

		Expect(altinn).To(HaveKeyWithValue("iss", server.URL+"/authentication/api/v1/openid/"))
		Expect(altinn).To(HaveKeyWithValue("urn:altinn:orgNumber", kartverket))
		Expect(altinn).To(HaveKeyWithValue("urn:altinn:authenticatemethod", "maskinporten"))
		Expect(altinn).To(HaveKeyWithValue("urn:altinn:authlevel", BeNumerically("==", 3)))
		Expect(altinn).NotTo(HaveKey("client_amr"))
		Expect(altinn["exp"].(float64) - float64(time.Now().Unix())).To(BeNumerically("~", 1800, 10))
	})

	DescribeTable("answers 401 without a Maskinporten token that has not expired",
		func(headers map[string]string) {
			answer := exchange(headers)

			Expect(answer.status).To(Equal(http.StatusUnauthorized))
			Expect(answer.mediaType()).To(Equal("application/problem+json"))
			Expect(answer.json()).To(HaveKeyWithValue("title", "Unauthorized"))
		},
		Entry("no token", map[string]string{}),
		Entry("an empty token", map[string]string{"Authorization": "Bearer "}),
		Entry("a token that is not a JWT", map[string]string{"Authorization": "Bearer not-a-jwt"}),
		Entry("an expired token", map[string]string{"Authorization": "Bearer " + maskinportenToken(
			func(c maskinportenClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() })}),
		Entry("a token without exp", map[string]string{"Authorization": "Bearer " + maskinportenToken(
			func(c maskinportenClaims) { delete(c, "exp") })}),
	)
})
