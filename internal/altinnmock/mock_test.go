package altinnmock_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Changing a resource directly in the mock", func() {
	It("adds a version that Altinn's API then returns, like a change in Altinn Studio", func() {
		Expect(mock.SetResource(resource("kv-test"))).To(Succeed())
		first, _ := mock.Resource("kv-test")
		changed := resource("kv-test")
		changed["status"] = "Active"

		Expect(mock.SetResource(changed)).To(Succeed())

		Expect(fetch("kv-test")).To(HaveKeyWithValue("status", "Active"))
		second, _ := mock.Resource("kv-test")
		Expect(second["versionId"]).To(BeNumerically(">", first["versionId"]))
		Expect(second["createdAt"]).To(Equal(first["createdAt"]))
	})

	It("keeps the resource as Altinn would, without Altinn's checks", func() {
		incomplete := map[string]any{"identifier": "kv-test", "somethingElse": "x"}

		Expect(mock.SetResource(incomplete)).To(Succeed())

		stored, found := mock.Resource("kv-test")
		Expect(found).To(BeTrue())
		Expect(stored).NotTo(HaveKey("somethingElse"))
		Expect(stored).To(HaveKeyWithValue("delegable", true))
	})

	It("refuses a resource without an identifier", func() {
		Expect(mock.SetResource(map[string]any{"title": texts("kv-test")})).To(HaveOccurred())
	})

	It("refuses a resource that is not JSON, and keeps none of it", func() {
		unencodable := resource("kv-test")
		unencodable["status"] = make(chan int)

		Expect(mock.SetResource(unencodable)).To(HaveOccurred())

		_, found := mock.Resource("kv-test")
		Expect(found).To(BeFalse())
	})

	It("finds no resource that was never made", func() {
		_, found := mock.Resource("kv-nothing")

		Expect(found).To(BeFalse())
	})
})
