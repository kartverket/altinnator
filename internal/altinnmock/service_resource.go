package altinnmock

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type serviceResourceField struct {
	name string
	// Altinn's value types are the only fields with a default
	defaultValue any
	// The name in Altinn's message when a PUT lacks the field. On a POST, Altinn does not check it
	requiredOnUpdate string
}

// ServiceResource's fields, in Altinn's order. versionId, createdAt and updatedAt are Altinn's to set
var serviceResourceFields = []serviceResourceField{
	{name: "identifier"},
	{name: "version"},
	{name: "title", requiredOnUpdate: "Title"},
	{name: "description", requiredOnUpdate: "Description"},
	{name: "rightDescription"},
	{name: "homepage"},
	{name: "status"},
	{name: "spatial"},
	{name: "contactPoints", requiredOnUpdate: "ContactPoints"},
	{name: "produces"},
	{name: "isPartOf"},
	{name: "thematicAreas"},
	{name: "resourceReferences"},
	{name: "delegable", defaultValue: true},
	{name: "visible", defaultValue: true},
	{name: "hasCompetentAuthority", requiredOnUpdate: "HasCompetentAuthority"},
	{name: "keywords"},
	{name: "accessListMode", defaultValue: "Disabled"},
	{name: "selfIdentifiedUserEnabled", defaultValue: false},
	{name: "enterpriseUserEnabled", defaultValue: false},
	{name: "resourceType", defaultValue: "Default"},
	{name: "availableForType"},
	{name: "authorizationReference"},
	{name: "consentTemplate"},
	{name: "consentText"},
	{name: "consentMetadata"},
	{name: "isOneTimeConsent", defaultValue: false},
}

// What Altinn keeps of a resource it is sent: the fields of its ServiceResource class, without nulls, and with the
// default of a value type that was left out. Altinn also takes field names in any case and enums as numbers, and
// refuses a value of the wrong type. The mock does not, since Altinnator sends what Altinn's API describes
func keepAsAltinn(body map[string]any) map[string]any {
	result := map[string]any{}
	for _, field := range serviceResourceFields {
		if value := body[field.name]; value != nil {
			result[field.name] = value
		} else if field.defaultValue != nil {
			result[field.name] = field.defaultValue
		}
	}
	return result
}

type validationMessages map[string][]string

func (m validationMessages) add(key, message string) { m[key] = append(m[key], message) }

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9_-]{4,}$`)
	languages         = []struct{ code, name string }{{"en", "english"}, {"nb", "bokmal"}, {"nn", "nynorsk"}}
)

// Altinn's checks of a resource, with Altinn's keys and messages. Only a new resource's identifier must match
// Altinn's pattern
func validateServiceResource(resource map[string]any, isNew bool, orgCode string) validationMessages {
	messages := validationMessages{}
	identifier := text(resource, "identifier")
	resourceType := text(resource, "resourceType")
	references := objects(resource, "resourceReferences")
	referenceTypeNames := make([]string, 0, len(references))
	for _, reference := range references {
		referenceTypeNames = append(referenceTypeNames, text(reference, "referenceType"))
	}
	if strings.HasPrefix(identifier, "se_") &&
		(!slices.Contains(referenceTypeNames, "ServiceCode") || !slices.Contains(referenceTypeNames, "ServiceEditionCode")) {
		messages.add("Identifier", "Invalid prefix. Only to be used for Altinn 2 services")
	}
	if strings.HasPrefix(identifier, "app_") && resourceType != "AltinnApp" && resourceType != "MigratedApp" {
		messages.add("Identifier", "Invalid Prefix. app_ is only for Altinn Studio apps")
	}
	if resourceType == "Default" {
		messages.add("ResourceType", "ResourceType needs to be set")
	}
	if !hasValidOwner(resource, orgCode) {
		messages.add("HasCompetentAuthority.Organization",
			"HasCompetentAuthority needs to be set with valid organization number and orgcode")
	}
	if isNew && !identifierPattern.MatchString(identifier) {
		messages.add("Identifier", "Invalid id. Only a-z and 0-9 is allowed together with _ and -.  Minimum 4 characters")
	}
	for _, language := range languages {
		if !hasLanguage(resource, "title", language.code) {
			messages.add("Title", fmt.Sprintf("Missing title in %s %s", language.name, language.code))
		}
	}
	if resource["delegable"] == true {
		for _, language := range languages {
			if !hasLanguage(resource, "rightDescription", language.code) {
				messages.add("RightDescription",
					fmt.Sprintf("Missing RightDescription in %s %s", language.name, language.code))
			}
		}
	}
	for _, language := range languages {
		if !hasLanguage(resource, "description", language.code) {
			messages.add("Description", fmt.Sprintf("Missing Description in %s %s", language.name, language.code))
		}
	}
	if resourceType == "MaskinportenSchema" && !slices.Contains(referenceTypeNames, "MaskinportenScope") {
		messages.add("ResourceReferences", "Missing maskinporten scopen for MaskinportenSchema resource")
	}
	return messages
}

func hasValidOwner(resource map[string]any, orgCode string) bool {
	authority, ok := resource["hasCompetentAuthority"].(map[string]any)
	if !ok {
		return false
	}
	code := text(authority, "orgcode")
	if strings.EqualFold(code, "ttd") {
		return true
	}
	if strings.TrimSpace(text(authority, "organization")) == "" || strings.TrimSpace(code) == "" {
		return false
	}
	return orgCode != "" && strings.EqualFold(code, orgCode)
}

func hasLanguage(resource map[string]any, field, language string) bool {
	texts, _ := resource[field].(map[string]any)
	return strings.TrimSpace(text(texts, language)) != ""
}

// Altinn's register of organisations, which it looks up a resource owner's org code in. The mock knows Kartverket and
// one other organisation
var orgCodes = map[string]string{"971040238": "kv", "981544315": "slf"}

func text(object map[string]any, field string) string {
	value, _ := object[field].(string)
	return value
}

func objects(object map[string]any, field string) []map[string]any {
	items, _ := object[field].([]any)
	var result []map[string]any
	for _, item := range items {
		if itemObject, ok := item.(map[string]any); ok {
			result = append(result, itemObject)
		}
	}
	return result
}
