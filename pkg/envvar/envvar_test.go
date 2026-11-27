package envvar_test

import (
	"os"
	"testing"

	"github.com/kartverket/altinnator/pkg/envvar"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEnvvar(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Envvar Suite")
}

// setEnv sets an environment variable for one spec, and restores it afterwards.
func setEnv(key, value string) {
	old, had := os.LookupEnv(key)
	Expect(os.Setenv(key, value)).To(Succeed())
	DeferCleanup(func() {
		if had {
			Expect(os.Setenv(key, old)).To(Succeed())
		} else {
			Expect(os.Unsetenv(key)).To(Succeed())
		}
	})
}

func unsetEnv(key string) {
	setEnv(key, "")
	Expect(os.Unsetenv(key)).To(Succeed())
}

var _ = Describe("Load", func() {
	BeforeEach(func() {
		setEnv("ALTINNATOR_ALTINN_PLATFORM_BASE_URL", "https://platform.tt02.altinn.no")
	})

	It("reads the Altinn platform base URL", func() {
		Expect(envvar.Load()).To(Succeed())

		Expect(envvar.Get().AltinnPlatformBaseURL).To(Equal("https://platform.tt02.altinn.no"))
	})

	It("requires the Altinn platform base URL", func() {
		unsetEnv("ALTINNATOR_ALTINN_PLATFORM_BASE_URL")

		Expect(envvar.Load()).To(MatchError(ContainSubstring("ALTINNATOR_ALTINN_PLATFORM_BASE_URL")))
	})

	It("refuses a blank Altinn platform base URL", func() {
		setEnv("ALTINNATOR_ALTINN_PLATFORM_BASE_URL", "  ")

		Expect(envvar.Load()).To(MatchError("ALTINNATOR_ALTINN_PLATFORM_BASE_URL must not be blank"))
	})
})
