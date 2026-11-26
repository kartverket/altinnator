package altinnmock_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/kartverket/altinnator/internal/altinnmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	kartverket   = "971040238"
	resourcePath = "/resourceregistry/api/v1/resource"
	writeScope   = "altinn:resourceregistry/resource.write"
	exchangePath = "/authentication/api/v1/exchange/maskinporten"
)

type response struct {
	status int
	header http.Header
	body   string
}

func (r response) json() map[string]any {
	var body map[string]any
	Expect(json.Unmarshal([]byte(r.body), &body)).To(Succeed(), r.body)
	return body
}

func (r response) mediaType() string {
	mediaType, _, _ := strings.Cut(r.header.Get("Content-Type"), ";")
	return mediaType
}

var (
	mock   *altinnmock.Mock
	server *httptest.Server
)

var _ = BeforeEach(func() {
	mock = altinnmock.New()
	server = httptest.NewServer(mock)
	DeferCleanup(server.Close)
})

func send(method, path string, body io.Reader, headers map[string]string) response {
	request, err := http.NewRequest(method, server.URL+path, body)
	Expect(err).NotTo(HaveOccurred())
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	answer, err := server.Client().Do(request)
	Expect(err).NotTo(HaveOccurred())
	defer func() { _ = answer.Body.Close() }()
	raw, err := io.ReadAll(answer.Body)
	Expect(err).NotTo(HaveOccurred())
	return response{answer.StatusCode, answer.Header, string(raw)}
}

func get(path string) response { return send(http.MethodGet, path, nil, nil) }

func fetch(id string) map[string]any {
	answer := get(resourcePath + "/" + id)
	Expect(answer.status).To(Equal(http.StatusOK), answer.body)
	return answer.json()
}

type maskinportenClaims map[string]any

// A Maskinporten token, the way Texas would hand it to Altinnator. The mock does not check signatures, so it has a
// made-up one
func maskinportenToken(change ...func(maskinportenClaims)) string {
	now := time.Now()
	claims := maskinportenClaims{
		"iss":             "https://test.maskinporten.no/",
		"scope":           writeScope,
		"consumer":        map[string]any{"authority": "iso6523-actorid-upis", "ID": "0192:" + kartverket},
		"consumer_prefix": "kartverk",
		"client_id":       "dev-client",
		"iat":             now.Unix(),
		"exp":             now.Add(2 * time.Minute).Unix(),
	}
	for _, c := range change {
		c(claims)
	}
	return encode(map[string]any{"alg": "RS256", "kid": "maskinporten"}) + "." + encode(claims) + ".c2lnbmF0dXJl"
}

func encode(value any) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func claimsOf(jwt string) map[string]any {
	raw, err := base64.RawURLEncoding.DecodeString(strings.Split(jwt, ".")[1])
	Expect(err).NotTo(HaveOccurred())
	var claims map[string]any
	Expect(json.Unmarshal(raw, &claims)).To(Succeed())
	return claims
}

func exchange(headers map[string]string) response {
	return send(http.MethodGet, exchangePath, nil, headers)
}

// An Altinn token from the mock, the way Altinnator gets one: a Maskinporten token, exchanged
func altinnToken(scope string) string {
	answer := exchange(map[string]string{
		"Authorization": "Bearer " + maskinportenToken(func(c maskinportenClaims) { c["scope"] = scope }),
	})
	Expect(answer.status).To(Equal(http.StatusOK), answer.body)
	return answer.body
}

func texts(id string) map[string]any {
	return map[string]any{"en": id + " en", "nb": id + " nb", "nn": id + " nn"}
}

// A MaskinportenSchema resource that Altinn accepts from Kartverket
func resource(id string) map[string]any {
	return map[string]any{
		"identifier":       id,
		"title":            texts(id),
		"description":      texts(id),
		"rightDescription": texts(id),
		"contactPoints":    []any{},
		"resourceType":     "MaskinportenSchema",
		"resourceReferences": []any{
			map[string]any{"referenceSource": "Default", "reference": "kartverk:test", "referenceType": "MaskinportenScope"},
		},
		"hasCompetentAuthority": map[string]any{"organization": kartverket, "orgcode": "kv"},
	}
}
