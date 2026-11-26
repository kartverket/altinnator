package altinnmock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

const resourcePath = "/resourceregistry/api/v1/resource"

// The routes of Altinn's Resource Registry
type resourceRegistry struct {
	registry *registry
}

func (rr *resourceRegistry) handleGet(w http.ResponseWriter, r *http.Request) {
	resource, ok := rr.registry.get(r.PathValue("id"))
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

// Altinn's ASP.NET answers its errors as ProblemDetails, but sends them as application/json like everything else
func writeNotFound(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotFound, "https://tools.ietf.org/html/rfc9110#section-15.5.5", "Not Found")
}

// Altinn's ProblemDetails, with its fields in Altinn's order
type problemDetails struct {
	Type    string `json:"type,omitempty"`
	Title   string `json:"title"`
	Status  int    `json:"status"`
	TraceID string `json:"traceId,omitempty"`
}

func writeProblemJSON(w http.ResponseWriter, status int, problemType, title string) {
	body, _ := json.Marshal(problemDetails{Type: problemType, Title: title, Status: status, TraceID: traceID()})
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeProblem(w http.ResponseWriter, status int, problemType, title string) {
	writeJSON(w, status, problemDetails{Type: problemType, Title: title, Status: status, TraceID: traceID()})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	raw, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func traceID() string {
	return "00-" + randomHex(16) + "-" + randomHex(8) + "-00"
}

func randomHex(bytes int) string {
	b := make([]byte, bytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
