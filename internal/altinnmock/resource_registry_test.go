package altinnmock_test

import (
	"net/http"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reading a resource", func() {
	It("finds a resource by its id, with its version and timestamps", func() {
		Expect(mock.SetResource(resource("kv-test"))).To(Succeed())

		stored := fetch("kv-test")

		Expect(stored["identifier"]).To(Equal("kv-test"))
		Expect(stored["versionId"]).To(BeNumerically(">", 0))
		Expect(stored["createdAt"]).To(MatchRegexp(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}\+00:00$`))
		Expect(stored["createdAt"]).To(Equal(stored["updatedAt"]))
	})

	It("answers an unknown id with a 404 ProblemDetails, sent as application/json", func() {
		answer := get(resourcePath + "/kv-nothing")

		Expect(answer.status).To(Equal(http.StatusNotFound))
		Expect(answer.mediaType()).To(Equal("application/json"))
		Expect(answer.json()).To(HaveKeyWithValue("title", "Not Found"))
	})

	It("matches the id exactly, case included", func() {
		Expect(mock.SetResource(resource("kv-case"))).To(Succeed())

		Expect(get(resourcePath + "/KV-CASE").status).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("Creating a resource", func() {
	It("answers 201 with a Location and no body, and the resource can then be read", func() {
		answer := create(resource("kv-test"))

		Expect(answer.status).To(Equal(http.StatusCreated))
		Expect(answer.header.Get("Location")).To(Equal(resourcePath + "/kv-test"))
		Expect(answer.body).To(BeEmpty())
		Expect(fetch("kv-test")["title"]).To(HaveKeyWithValue("nb", "kv-test nb"))
	})

	It("sets the version and timestamps itself, counting versions across resources", func() {
		body := resource("kv-first")
		body["versionId"] = 1000
		body["createdAt"] = "2000-01-01T00:00:00+00:00"
		create(body)
		create(resource("kv-second"))

		first, second := fetch("kv-first"), fetch("kv-second")

		Expect(second["versionId"]).To(Equal(first["versionId"].(float64) + 1))
		Expect(first["createdAt"]).To(Equal(first["updatedAt"]))
		Expect(first["createdAt"]).NotTo(HavePrefix("2000"))
	})

	It("keeps only Altinn's fields, drops nulls and fills in the defaults Altinn has", func() {
		body := resource("kv-test")
		body["somethingElse"] = "x"
		body["homepage"] = nil
		create(body)

		stored := fetch("kv-test")

		Expect(stored).NotTo(HaveKey("somethingElse"))
		Expect(stored).NotTo(HaveKey("homepage"))
		Expect(stored).To(HaveKeyWithValue("delegable", true))
		Expect(stored).To(HaveKeyWithValue("visible", true))
		Expect(stored).To(HaveKeyWithValue("accessListMode", "Disabled"))
	})

	It("refuses a body that is not a JSON object", func() {
		answer := writeJSON(http.MethodPost, resourcePath, []any{}, altinnToken(writeScope))

		Expect(answer.status).To(Equal(http.StatusBadRequest))
		Expect(answer.errors()).To(HaveKey("$"))
	})

	It("needs an Altinn token, and one with a scope to write", func() {
		noToken := send(http.MethodPost, resourcePath, strings.NewReader("{}"),
			map[string]string{"Content-Type": "application/json"})
		maskinporten := writeJSON(http.MethodPost, resourcePath, resource("kv-test"), maskinportenToken())
		readOnly := writeJSON(http.MethodPost, resourcePath, resource("kv-test"),
			altinnToken("altinn:resourceregistry/resource.read"))

		Expect(noToken.status).To(Equal(http.StatusUnauthorized))
		Expect(maskinporten.status).To(Equal(http.StatusUnauthorized))
		Expect(readOnly.status).To(Equal(http.StatusForbidden))
		Expect(readOnly.body).To(BeEmpty())
	})

	It("answers a body sent as something other than JSON with 415", func() {
		answer := send(http.MethodPost, resourcePath, strings.NewReader("{}"), map[string]string{
			"Authorization": "Bearer " + altinnToken(writeScope),
			"Content-Type":  "text/plain",
		})

		Expect(answer.status).To(Equal(http.StatusUnsupportedMediaType))
	})

	It("forbids another organisation's resource", func() {
		answer := create(resource("kv-test", ownedBy(otherOrg, otherOrgCode)))

		Expect(answer.status).To(Equal(http.StatusForbidden))
		Expect(answer.body).To(BeEmpty())
	})

	It("needs every scope to start with Kartverket's prefix, unless the token has the admin scope", func() {
		answer := create(resource("kv-test", withScope("ldir:test")))
		asAdmin := writeJSON(http.MethodPost, resourcePath, resource("kv-test", withScope("ldir:test")),
			altinnToken(adminScope))

		Expect(answer.status).To(Equal(http.StatusUnauthorized))
		Expect(answer.errors()).To(Equal(map[string][]string{"InvalidPrefix": {"ldir:test"}}))
		Expect(answer.json()).To(HaveKeyWithValue("detail", "Not authorized for creating resource with the listed scopes"))
		Expect(asAdmin.status).To(Equal(http.StatusCreated))
	})

	It("answers an identifier that is taken with 409, as a JSON string", func() {
		create(resource("kv-test"))

		answer := create(resource("kv-test"))

		Expect(answer.status).To(Equal(http.StatusConflict))
		Expect(answer.body).To(Equal(`"The Resource already exist: kv-test"`))
	})
})

var _ = Describe("Updating a resource", func() {
	BeforeEach(func() {
		Expect(create(resource("kv-test")).status).To(Equal(http.StatusCreated))
	})

	It("adds a new version, keeps createdAt and answers 200 without a body", func() {
		first := fetch("kv-test")
		body := resource("kv-test")
		body["status"] = "Active"

		answer := update("kv-test", body)
		second := fetch("kv-test")

		Expect(answer.status).To(Equal(http.StatusOK))
		Expect(answer.body).To(BeEmpty())
		Expect(second).To(HaveKeyWithValue("status", "Active"))
		Expect(second["versionId"]).To(BeNumerically(">", first["versionId"]))
		Expect(second["createdAt"]).To(Equal(first["createdAt"]))
	})

	It("adds a new version even when nothing changed", func() {
		first := fetch("kv-test")

		update("kv-test", resource("kv-test"))

		Expect(fetch("kv-test")["versionId"]).To(BeNumerically(">", first["versionId"]))
	})

	It("replaces the whole resource, so an omitted field is gone", func() {
		body := resource("kv-test")
		body["homepage"] = "https://kartverket.no"
		update("kv-test", body)

		update("kv-test", resource("kv-test"))

		Expect(fetch("kv-test")).NotTo(HaveKey("homepage"))
	})

	It("answers an unknown id with 404, and refuses an id that differs from the body's", func() {
		Expect(update("kv-nothing", resource("kv-nothing")).status).To(Equal(http.StatusNotFound))

		mismatch := update("kv-test", resource("kv-other"))

		Expect(mismatch.status).To(Equal(http.StatusBadRequest))
		Expect(mismatch.body).To(Equal(`"Id in path does not match ID in resource"`))
	})

	It("needs title, description, contact points and owner", func() {
		body := resource("kv-test")
		delete(body, "title")
		delete(body, "contactPoints")

		answer := update("kv-test", body)

		Expect(answer.errors()).To(Equal(map[string][]string{
			"Title":         {"The Title field is required."},
			"ContactPoints": {"The ContactPoints field is required."},
		}))
	})

	It("only checks the owner in the body, so it can change a resource another organisation owns", func() {
		Expect(mock.SetResource(resource(otherResource, ownedBy(otherOrg, otherOrgCode)))).To(Succeed())

		answer := update(otherResource, resource(otherResource))

		Expect(answer.status).To(Equal(http.StatusOK))
		Expect(fetch(otherResource)["hasCompetentAuthority"]).To(HaveKeyWithValue("organization", kartverket))
	})

	It("keeps a withdrawn resource", func() {
		body := resource("kv-test")
		delete(body, "rightDescription")
		body["delegable"] = false
		body["visible"] = false
		body["status"] = "Withdrawn"

		Expect(update("kv-test", body).status).To(Equal(http.StatusOK))

		stored := fetch("kv-test")
		Expect(stored).To(HaveKeyWithValue("status", "Withdrawn"))
		Expect(stored).To(HaveKeyWithValue("visible", false))
	})
})
