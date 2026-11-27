package altinnmock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"slices"
	"strings"
)

const (
	resourcePath = "/resourceregistry/api/v1/resource"
	writeScope   = "altinn:resourceregistry/resource.write"
	adminScope   = "altinn:resourceregistry/resource.admin"
)

// The routes of Altinn's Resource Registry. Like in Altinn, anyone may read, and a write needs an Altinn token with
// the scope to write or the admin scope
type resourceRegistry struct {
	registry *registry
	tokens   *tokens
}

func (rr *resourceRegistry) handleGet(w http.ResponseWriter, r *http.Request) {
	resource, ok := rr.registry.get(r.PathValue("id"))
	if !ok {
		writeNotFound(w)
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (rr *resourceRegistry) handleCreate(w http.ResponseWriter, r *http.Request) {
	claims, ok := rr.writer(w, r)
	if !ok {
		return
	}
	resource, ok := receiveResource(w, r)
	if !ok {
		return
	}
	id, hasID := resource["identifier"].(string)
	// Altinn throws on these before it gets to say what is wrong
	if !hasID || resource["hasCompetentAuthority"] == nil {
		writeInternalError(w)
		return
	}
	if !passesChecks(w, resource, true) || !isAllowed(w, resource, claims) {
		return
	}
	if !rr.registry.create(resource) {
		writeJSON(w, http.StatusConflict, "The Resource already exist: "+id)
		return
	}
	w.Header().Set("Location", resourcePath+"/"+id)
	w.WriteHeader(http.StatusCreated)
}

func (rr *resourceRegistry) handleUpdate(w http.ResponseWriter, r *http.Request) {
	claims, ok := rr.writer(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !hasMediaType(r, "application/json") {
		writeUnsupportedMediaType(w)
		return
	}
	if _, exists := rr.registry.get(id); !exists {
		writeNotFound(w)
		return
	}
	resource, ok := receiveResource(w, r)
	if !ok {
		return
	}
	if resource["identifier"] != id {
		writeJSON(w, http.StatusBadRequest, "Id in path does not match ID in resource")
		return
	}
	missing := validationMessages{}
	for _, field := range serviceResourceFields {
		if field.requiredOnUpdate != "" && resource[field.name] == nil {
			missing.add(field.requiredOnUpdate, fmt.Sprintf("The %s field is required.", field.requiredOnUpdate))
		}
	}
	if len(missing) > 0 {
		writeValidationProblem(w, missing)
		return
	}
	if !passesChecks(w, resource, false) || !isAllowed(w, resource, claims) {
		return
	}
	rr.registry.put(resource)
	w.WriteHeader(http.StatusOK)
}

// Altinn takes a policy with POST and with PUT, and does the same with both
func (rr *resourceRegistry) handlePolicy(w http.ResponseWriter, r *http.Request) {
	claims, ok := rr.writer(w, r)
	if !ok {
		return
	}
	if !hasMediaType(r, "multipart/form-data") {
		writeUnsupportedMediaType(w)
		return
	}
	id := r.PathValue("id")
	policy, err := policyFile(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, "The policy file can not be null")
		return
	}
	resource, exists := rr.registry.get(id)
	if !exists {
		writeJSON(w, http.StatusBadRequest, "Unknown resource")
		return
	}
	// Unlike a write of the resource, Altinn checks the owner it has, not the one in the request
	if !hasWriteAccess(ownerOf(resource), claims) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	problem, err := policyProblem(policy, id)
	if err != nil {
		writeInternalError(w)
		return
	}
	if problem != "" {
		writeJSON(w, http.StatusBadRequest, problem)
		return
	}
	w.Header().Set("Location", id+"/policy")
	w.WriteHeader(http.StatusCreated)
}

func policyFile(r *http.Request) ([]byte, error) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	files := r.MultipartForm.File["policyFile"]
	if len(files) == 0 {
		return nil, errors.New("no policy file")
	}
	file, err := files[0].Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(file)
}

func (rr *resourceRegistry) writer(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	claims := rr.tokens.altinnClaims(bearerToken(r))
	if claims == nil {
		w.WriteHeader(http.StatusUnauthorized)
		return nil, false
	}
	scopes := scopesOf(claims)
	if !slices.Contains(scopes, writeScope) && !slices.Contains(scopes, adminScope) {
		w.WriteHeader(http.StatusForbidden)
		return nil, false
	}
	return claims, true
}

func receiveResource(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	if !hasMediaType(r, "application/json") {
		writeUnsupportedMediaType(w)
		return nil, false
	}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil || body == nil {
		writeValidationProblem(w, validationMessages{
			"$": {"The JSON value could not be converted to ServiceResource. Path: $"},
		})
		return nil, false
	}
	return keepAsAltinn(body), true
}

// Altinn checks the resource before it checks who may write it
func passesChecks(w http.ResponseWriter, resource map[string]any, isNew bool) bool {
	if messages := validateServiceResource(resource, isNew, orgCodes[ownerOf(resource)]); len(messages) > 0 {
		writeValidationProblem(w, messages)
		return false
	}
	return true
}

func isAllowed(w http.ResponseWriter, resource map[string]any, claims map[string]any) bool {
	// Altinn checks the owner in the request, also when it changes a resource another organisation owns
	if !hasWriteAccess(ownerOf(resource), claims) {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	if unauthorized := scopesWithoutPrefix(resource, claims); len(unauthorized) > 0 {
		writeJSON(w, http.StatusUnauthorized, problemDetails{
			Title:  "Unauthorized",
			Status: http.StatusUnauthorized,
			Detail: "Not authorized for creating resource with the listed scopes",
			Errors: validationMessages{"InvalidPrefix": unauthorized},
		})
		return false
	}
	return true
}

// Altinn's own check, in its default setup: the admin scope may write anyone's resources, and otherwise the token's
// consumer must be the owner
func hasWriteAccess(owner string, claims map[string]any) bool {
	scope, _ := claims["scope"].(string)
	if strings.Contains(strings.ToLower(scope), adminScope) {
		return true
	}
	consumer, _ := claims["consumer"].(map[string]any)
	consumerID, _ := consumer["ID"].(string)
	organization, isOrganization := strings.CutPrefix(consumerID, "0192:")
	return isOrganization && strings.EqualFold(organization, owner)
}

// Altinn's check of scope prefixes in its default setup: unless the token has the admin scope, every Maskinporten
// scope of the resource must start with one of the token's consumer_prefix values and a colon
func scopesWithoutPrefix(resource map[string]any, claims map[string]any) []string {
	var scopes []string
	for _, reference := range objects(resource, "resourceReferences") {
		if text(reference, "referenceType") == "MaskinportenScope" && text(reference, "reference") != "" {
			scopes = append(scopes, text(reference, "reference"))
		}
	}
	if len(scopes) == 0 || slices.Contains(scopesOf(claims), adminScope) {
		return nil
	}
	var prefixes []string
	switch prefix := claims["consumer_prefix"].(type) {
	case string:
		prefixes = []string{prefix}
	case []any:
		for _, p := range prefix {
			if s, ok := p.(string); ok {
				prefixes = append(prefixes, s)
			}
		}
	}
	var unauthorized []string
	for _, scope := range scopes {
		if !slices.ContainsFunc(prefixes, func(prefix string) bool { return strings.HasPrefix(scope, prefix+":") }) {
			unauthorized = append(unauthorized, scope)
		}
	}
	return unauthorized
}

func scopesOf(claims map[string]any) []string {
	scope, _ := claims["scope"].(string)
	return strings.Split(scope, " ")
}

func ownerOf(resource map[string]any) string {
	authority, _ := resource["hasCompetentAuthority"].(map[string]any)
	return text(authority, "organization")
}

func hasMediaType(r *http.Request, want string) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == want
}

// Altinn's ASP.NET answers its errors as ProblemDetails, but sends them as application/json like everything else
func writeNotFound(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotFound, "https://tools.ietf.org/html/rfc9110#section-15.5.5", "Not Found")
}

func writeUnsupportedMediaType(w http.ResponseWriter) {
	writeProblem(w, http.StatusUnsupportedMediaType, "https://tools.ietf.org/html/rfc9110#section-15.5.16",
		"Unsupported Media Type")
}

// Altinn's ProblemDetails, with its fields in Altinn's order
type problemDetails struct {
	Type    string             `json:"type,omitempty"`
	Title   string             `json:"title"`
	Status  int                `json:"status"`
	Detail  string             `json:"detail,omitempty"`
	Errors  validationMessages `json:"errors,omitempty"`
	TraceID string             `json:"traceId,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, problemType, title string) {
	writeJSON(w, status, problemDetails{Type: problemType, Title: title, Status: status, TraceID: traceID()})
}

func writeValidationProblem(w http.ResponseWriter, messages validationMessages) {
	writeJSON(w, http.StatusBadRequest, problemDetails{
		Type:    "https://tools.ietf.org/html/rfc9110#section-15.5.1",
		Title:   "One or more validation errors occurred.",
		Status:  http.StatusBadRequest,
		Errors:  messages,
		TraceID: traceID(),
	})
}

// What Altinn answers when it throws: the only error it sends as application/problem+json
func writeInternalError(w http.ResponseWriter) {
	writeProblemJSON(w, http.StatusInternalServerError, "https://tools.ietf.org/html/rfc9110#section-15.6.1",
		"An error occurred while processing your request.")
}

func writeProblemJSON(w http.ResponseWriter, status int, problemType, title string) {
	body, _ := json.Marshal(problemDetails{Type: problemType, Title: title, Status: status, TraceID: traceID()})
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
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
