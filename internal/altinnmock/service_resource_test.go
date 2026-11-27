package altinnmock_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Altinn's checks of a resource", func() {
	It("gives all of Altinn's checks of a new resource in one answer, with Altinn's keys and messages", func() {
		answer := create(map[string]any{
			"identifier":            "KV",
			"title":                 map[string]any{"en": "Title"},
			"resourceType":          "MaskinportenSchema",
			"hasCompetentAuthority": map[string]any{"organization": kartverket, "orgcode": "xx"},
		})

		Expect(answer.status).To(Equal(http.StatusBadRequest))
		Expect(answer.errors()).To(Equal(map[string][]string{
			"HasCompetentAuthority.Organization": {
				"HasCompetentAuthority needs to be set with valid organization number and orgcode",
			},
			"Identifier": {"Invalid id. Only a-z and 0-9 is allowed together with _ and -.  Minimum 4 characters"},
			"Title":      {"Missing title in bokmal nb", "Missing title in nynorsk nn"},
			"RightDescription": {
				"Missing RightDescription in english en",
				"Missing RightDescription in bokmal nb",
				"Missing RightDescription in nynorsk nn",
			},
			"Description": {
				"Missing Description in english en",
				"Missing Description in bokmal nb",
				"Missing Description in nynorsk nn",
			},
			"ResourceReferences": {"Missing maskinporten scopen for MaskinportenSchema resource"},
		}))
	})

	It("checks only a new resource's identifier against Altinn's pattern", func() {
		Expect(mock.SetResource(resource("KV_OLD"))).To(Succeed())

		Expect(update("KV_OLD", resource("KV_OLD")).status).To(Equal(http.StatusOK))
	})

	It("needs no right description for a resource that cannot be delegated", func() {
		body := resource("kv-test")
		delete(body, "rightDescription")
		body["delegable"] = false

		Expect(create(body).status).To(Equal(http.StatusCreated))
	})

	It("refuses an organisation number that is not 9 digits", func() {
		answer := create(resource("kv-test", ownedBy("97104023", "kv")))

		Expect(answer.status).To(Equal(http.StatusBadRequest))
		Expect(answer.errors()).To(HaveKey("HasCompetentAuthority.Organization"))
	})

	It("checks the resource before it forbids another organisation's", func() {
		invalid := resource("kv-test", ownedBy(otherOrg, otherOrgCode))
		delete(invalid, "title")

		Expect(create(invalid).status).To(Equal(http.StatusBadRequest))
	})
})
