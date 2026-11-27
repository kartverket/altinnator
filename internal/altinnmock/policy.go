package altinnmock

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
)

const (
	xacmlNamespace    = "urn:oasis:names:tc:xacml:3.0:core:schema:wd-17"
	resourceCategory  = "urn:oasis:names:tc:xacml:3.0:attribute-category:resource"
	resourceAttribute = "urn:altinn:resource"
)

// A policy Altinn's XACML parser cannot read. Altinn answers it with 500
var errUnreadablePolicy = errors.New("unreadable policy")

type element struct {
	namespace  string
	name       string
	attributes map[string]string
	children   []*element
	text       strings.Builder
}

// Altinn's check of a resource's XACML policy: it must be a policy Altinn can read, with at least one rule, and every
// rule must be about the resource and no other. Altinn does not check the rules' subjects or actions
func policyProblem(policy []byte, id string) (string, error) {
	rules, err := rulesOf(policy)
	if err != nil {
		return "", err
	}
	for _, rule := range rules {
		var resources []string
		for _, match := range rule.descendants("Match") {
			designators := match.descendants("AttributeDesignator")
			values := match.descendants("AttributeValue")
			if len(designators) == 0 || len(values) == 0 {
				continue
			}
			designator := designators[0]
			if designator.attributes["Category"] == resourceCategory &&
				designator.attributes["AttributeId"] == resourceAttribute {
				resources = append(resources, values[0].text.String())
			}
		}
		for _, resource := range resources {
			if resource != id {
				return "Policy not accepted: Contains rules for a different registry resource", nil
			}
		}
		if len(resources) == 0 {
			return "Policy not accepted: Contains rule without reference to registry resource id", nil
		}
	}
	return "", nil
}

func rulesOf(policy []byte) ([]*element, error) {
	root, err := parse(policy)
	if err != nil {
		return nil, err
	}
	if root.namespace != xacmlNamespace || root.name != "Policy" ||
		root.attributes["PolicyId"] == "" || root.attributes["RuleCombiningAlgId"] == "" {
		return nil, errUnreadablePolicy
	}
	rules := root.childrenNamed("Rule")
	if len(rules) == 0 {
		return nil, errUnreadablePolicy
	}
	for _, rule := range rules {
		if rule.attributes["RuleId"] == "" || !isEffect(rule.attributes["Effect"]) {
			return nil, errUnreadablePolicy
		}
	}
	for _, anyOf := range root.descendants("AnyOf") {
		if len(anyOf.childrenNamed("AllOf")) == 0 {
			return nil, errUnreadablePolicy
		}
	}
	for _, allOf := range root.descendants("AllOf") {
		if len(allOf.childrenNamed("Match")) == 0 {
			return nil, errUnreadablePolicy
		}
	}
	return rules, nil
}

// Altinn parses the effect as an enum: Permit or Deny in any case, or the enum's number
func isEffect(value string) bool {
	value = strings.TrimSpace(value)
	_, err := strconv.Atoi(value)
	return strings.EqualFold(value, "Permit") || strings.EqualFold(value, "Deny") || err == nil
}

func parse(document []byte) (*element, error) {
	decoder := xml.NewDecoder(bytes.NewReader(document))
	var root *element
	var open []*element
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errUnreadablePolicy
		}
		switch t := token.(type) {
		case xml.StartElement:
			e := &element{namespace: t.Name.Space, name: t.Name.Local, attributes: map[string]string{}}
			for _, attribute := range t.Attr {
				if attribute.Name.Space == "" {
					e.attributes[attribute.Name.Local] = attribute.Value
				}
			}
			if len(open) == 0 {
				root = e
			} else {
				parent := open[len(open)-1]
				parent.children = append(parent.children, e)
			}
			open = append(open, e)
		case xml.EndElement:
			open = open[:len(open)-1]
		case xml.CharData:
			if len(open) > 0 {
				open[len(open)-1].text.Write(t)
			}
		}
	}
	if root == nil {
		return nil, errUnreadablePolicy
	}
	return root, nil
}

func (e *element) childrenNamed(name string) []*element {
	var result []*element
	for _, child := range e.children {
		if child.namespace == xacmlNamespace && child.name == name {
			result = append(result, child)
		}
	}
	return result
}

func (e *element) descendants(name string) []*element {
	var result []*element
	for _, child := range e.children {
		if child.namespace == xacmlNamespace && child.name == name {
			result = append(result, child)
		}
		result = append(result, child.descendants(name)...)
	}
	return result
}
