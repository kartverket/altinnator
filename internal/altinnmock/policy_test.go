package altinnmock_test

import (
	"fmt"
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func match(resource string) string {
	return fmt.Sprintf(`<xacml:Match MatchId="urn:oasis:names:tc:xacml:1.0:function:string-equal">
  <xacml:AttributeValue DataType="http://www.w3.org/2001/XMLSchema#string">%s</xacml:AttributeValue>
  <xacml:AttributeDesignator AttributeId="urn:altinn:resource" `+
		`Category="urn:oasis:names:tc:xacml:3.0:attribute-category:resource" `+
		`DataType="http://www.w3.org/2001/XMLSchema#string" MustBePresent="false"/>
</xacml:Match>`, resource)
}

func target(allOf string) string {
	return "<xacml:Target><xacml:AnyOf><xacml:AllOf>" + allOf + "</xacml:AllOf></xacml:AnyOf></xacml:Target>"
}

// A rule about none of the resources has an empty target, as Altinn's parser needs a Match in every AllOf
func ruleWith(attributes string, resources ...string) string {
	body := "<xacml:Target/>"
	if len(resources) > 0 {
		matches := make([]string, 0, len(resources))
		for _, resource := range resources {
			matches = append(matches, match(resource))
		}
		body = target(strings.Join(matches, "\n"))
	}
	return "<xacml:Rule " + attributes + ">" + body + "</xacml:Rule>"
}

func rule(resources ...string) string { return ruleWith(`RuleId="rule" Effect="Permit"`, resources...) }

func policy(rules ...string) *string {
	document := `<?xml version="1.0" encoding="utf-8"?>
<xacml:Policy xmlns:xacml="urn:oasis:names:tc:xacml:3.0:core:schema:wd-17" PolicyId="urn:altinn:policyid:1" ` +
		`Version="1.0" RuleCombiningAlgId="urn:oasis:names:tc:xacml:3.0:rule-combining-algorithm:deny-overrides">
  <xacml:Target/>
  ` + strings.Join(rules, "\n") + `
</xacml:Policy>`
	return &document
}

func upload(id string, policy *string) response {
	return uploadPolicy(http.MethodPost, id, policy, altinnToken(writeScope))
}

var _ = Describe("Setting a policy", func() {
	BeforeEach(func() {
		Expect(mock.SetResource(resource("kv-test"))).To(Succeed())
		Expect(mock.SetResource(resource(otherResource, ownedBy(otherOrg, otherOrgCode)))).To(Succeed())
	})

	It("accepts a policy whose rules are all about the resource, with POST and PUT", func() {
		posted := upload("kv-test", policy(rule("kv-test")))
		put := uploadPolicy(http.MethodPut, "kv-test", policy(rule("kv-test"), rule("kv-test")), altinnToken(writeScope))

		Expect(posted.status).To(Equal(http.StatusCreated))
		Expect(posted.header.Get("Location")).To(Equal("kv-test/policy"))
		Expect(posted.body).To(BeEmpty())
		Expect(put.status).To(Equal(http.StatusCreated))
	})

	It("refuses a rule about another resource, or about none", func() {
		other := upload("kv-test", policy(rule("kv-test"), rule("kv-test", "kv-other")))
		none := upload("kv-test", policy(rule("kv-test"), rule()))

		Expect(other.status).To(Equal(http.StatusBadRequest))
		Expect(other.body).To(Equal(`"Policy not accepted: Contains rules for a different registry resource"`))
		Expect(none.status).To(Equal(http.StatusBadRequest))
		Expect(none.body).To(Equal(`"Policy not accepted: Contains rule without reference to registry resource id"`))
	})

	It("compares the resource in a rule exactly, like Altinn, so spaces around it make it another resource", func() {
		answer := upload("kv-test", policy(rule(" kv-test ")))

		Expect(answer.body).To(Equal(`"Policy not accepted: Contains rules for a different registry resource"`))
	})

	It("takes an effect in any case", func() {
		answer := upload("kv-test", policy(ruleWith(`RuleId="rule" Effect="permit"`, "kv-test")))

		Expect(answer.status).To(Equal(http.StatusCreated))
	})

	DescribeTable("fails like Altinn, with 500, on a policy Altinn cannot read",
		func(policy *string) {
			Expect(upload("kv-test", policy).status).To(Equal(http.StatusInternalServerError))
		},
		Entry("not XML", func() *string { s := "not xml"; return &s }()),
		Entry("a policy without rules", policy()),
		Entry("a rule without RuleId", policy(ruleWith(`Effect="Permit"`, "kv-test"))),
		Entry("a rule without Effect", policy(ruleWith(`RuleId="rule"`, "kv-test"))),
		Entry("an effect that is not Permit or Deny", policy(ruleWith(`RuleId="rule" Effect="Allow"`, "kv-test"))),
		Entry("an AllOf without a Match", policy(`<xacml:Rule RuleId="rule" Effect="Permit">`+target("")+`</xacml:Rule>`)),
		Entry("an AnyOf without an AllOf", policy(
			`<xacml:Rule RuleId="rule" Effect="Permit"><xacml:Target><xacml:AnyOf/></xacml:Target></xacml:Rule>`)),
	)

	It("refuses a request without a file, or for an unknown resource", func() {
		noFile := upload("kv-test", nil)
		unknown := upload("kv-nothing", policy(rule("kv-nothing")))

		Expect(noFile.body).To(Equal(`"The policy file can not be null"`))
		Expect(unknown.status).To(Equal(http.StatusBadRequest))
		Expect(unknown.body).To(Equal(`"Unknown resource"`))
	})

	It("needs the owner Altinn has to be the token's, unless the token has the admin scope", func() {
		forbidden := upload(otherResource, policy(rule(otherResource)))
		asAdmin := uploadPolicy(http.MethodPost, otherResource, policy(rule(otherResource)), altinnToken(adminScope))

		Expect(forbidden.status).To(Equal(http.StatusForbidden))
		Expect(asAdmin.status).To(Equal(http.StatusCreated))
	})

	It("needs a token with a scope to write", func() {
		Expect(uploadPolicy(http.MethodPost, "kv-test", policy(rule("kv-test")), "nope").status).
			To(Equal(http.StatusUnauthorized))
	})
})
