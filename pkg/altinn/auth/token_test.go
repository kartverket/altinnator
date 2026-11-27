package auth

import (
	"bytes"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var _ = Describe("Token", func() {
	token := Token{value: "secret-token", expiresAt: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)}

	It("does not show its value when formatted", func() {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			for _, value := range []any{token, &token} {
				Expect(fmt.Sprintf(format, value)).NotTo(ContainSubstring("secret-token"), format)
			}
		}
		Expect(token.String()).To(Equal("Token(***, expires 2026-11-23T12:00:00Z)"))
	})

	It("does not show its value in the log", func() {
		var out bytes.Buffer
		logger := zap.New(zap.WriteTo(&out))

		logger.Info("loaded", "token", token)

		Expect(out.String()).To(ContainSubstring("Token(***, expires 2026-11-23T12:00:00Z)"))
		Expect(out.String()).NotTo(ContainSubstring("secret-token"))
	})

	It("can be made with NewToken, and still hides its value", func() {
		made := NewToken("secret-token", time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))

		Expect(made.Reveal()).To(Equal("secret-token"))
		Expect(made).To(Equal(token))
		Expect(made.String()).To(Equal("Token(***, expires 2026-11-23T12:00:00Z)"))
	})
})
