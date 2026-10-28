package governance

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
)

var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,253}[a-z0-9])?$`)

func oneOf(v string, values ...string) bool {
	for _, s := range values {
		if s == v {
			return true
		}
	}
	return false
}
func Validate(kind string, p Payload, now time.Time) error {
	bad := func(s string) error { return fmt.Errorf("%w: %s", ErrInvalid, s) }
	b, _ := json.Marshal(p)
	if len(b) > 65536 {
		return bad("metadata exceeds 64 KiB")
	}
	if len(p.Description) > 4000 || len(p.Purpose) > 2000 || len(p.Name) > 255 {
		return bad("text is too long")
	}
	if kind != "request" && !(kind == "tool" && regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,255}$`).MatchString(p.Name)) && !namePattern.MatchString(p.Name) {
		return bad("name must be a lowercase DNS name")
	}
	if p.Endpoint != "" {
		u, e := url.Parse(p.Endpoint)
		if e != nil || !oneOf(u.Scheme, "http", "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return bad("endpoint must be an http(s) URL without credentials, query or fragment")
		}
	}
	switch kind {
	case "agent":
		if !oneOf(p.RegistrationScope, "", "internal", "outside_mb") {
			return bad("invalid registration scope")
		}
		if strings.TrimSpace(p.Purpose) == "" || strings.TrimSpace(p.Description) == "" || !oneOf(p.Risk, "low", "medium", "high") {
			return bad("purpose, business description and risk are required")
		}
	case "instance":
		if !oneOf(p.Environment, "dev", "test", "prod") || !oneOf(p.Location, "platform", "unit_infrastructure", "workstation") {
			return bad("invalid environment or execution location")
		}
	case "server":
		if p.Description == "" {
			return bad("server description is required")
		}
	case "tool":
		if p.Description == "" || p.Snapshot == "" || len(p.Snapshot) > 255 || !oneOf(p.Classification, "read", "write") || !oneOf(p.Risk, "low", "medium", "high") {
			return bad("tool description, snapshot, classification and risk are required")
		}
		if len(p.InputSchema) == 0 || len(p.InputSchema) > 32768 {
			return bad("schema is required and must fit 32 KiB")
		}
		var s jsonschema.Schema
		if e := json.Unmarshal(p.InputSchema, &s); e != nil {
			return bad("invalid JSON schema syntax")
		}
		if e := schemaKeywords(&s, 0); e != nil {
			return bad(e.Error())
		}
		// Resolving validates schema keywords/references without contacting a network.
		if _, e := s.Resolve(&jsonschema.ResolveOptions{}); e != nil {
			return bad("invalid or externally referenced JSON schema")
		}
	case "request":
		if !oneOf(p.RequestType, "agent", "access") || p.Purpose == "" {
			return bad("request type and purpose are required")
		}
		if p.RequestType == "access" {
			if len(p.Operations) == 0 || len(p.Operations) > 20 || len(p.Constraints) == 0 || p.ExpiresAt == nil || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.AddDate(1, 0, 0)) {
				return bad("operations, constraints and future expiry within one year are required")
			}
			for _, op := range p.Operations {
				if strings.TrimSpace(op) == "" || len(op) > 100 {
					return bad("invalid operation")
				}
			}
			for k, v := range p.Constraints {
				if k == "" || len(k) > 100 || v == "" || len(v) > 1000 {
					return bad("invalid constraint")
				}
			}
		}
	default:
		return bad("unsupported kind")
	}
	return nil
}

// CleanPayload prevents unused fields from being smuggled into persisted objects.
func CleanPayload(kind string, p Payload) Payload {
	switch kind {
	case "agent":
		return Payload{RegistrationScope: p.RegistrationScope, Name: p.Name, Namespace: p.Namespace, Tag: p.Tag, Purpose: p.Purpose, Description: p.Description, Risk: p.Risk}
	case "server":
		return Payload{Name: p.Name, Namespace: p.Namespace, Tag: p.Tag, Description: p.Description, Endpoint: p.Endpoint}
	case "instance":
		return Payload{Name: p.Name, Environment: p.Environment, Location: p.Location, Endpoint: p.Endpoint, Identity: p.Identity}
	case "tool":
		return Payload{Name: p.Name, Description: p.Description, InputSchema: p.InputSchema, Snapshot: p.Snapshot, Classification: p.Classification, Risk: p.Risk}
	case "request":
		return Payload{RequestType: p.RequestType, Purpose: p.Purpose, Operations: p.Operations, Constraints: p.Constraints, ExpiresAt: p.ExpiresAt}
	}
	return Payload{}
}

// The existing jsonschema library resolves references but does not yet validate
// type names or non-negative limits. Check those locally without a network loader.
func schemaKeywords(s *jsonschema.Schema, depth int) error {
	if s == nil {
		return nil
	}
	if depth > 32 {
		return fmt.Errorf("schema nesting exceeds 32 levels")
	}
	types := s.Types
	if s.Type != "" {
		types = append(types, s.Type)
	}
	seen := map[string]bool{}
	for _, typ := range types {
		if !oneOf(typ, "null", "boolean", "object", "array", "number", "integer", "string") || seen[typ] {
			return fmt.Errorf("invalid schema type")
		}
		seen[typ] = true
	}
	for _, n := range []*int{s.MinLength, s.MaxLength, s.MinItems, s.MaxItems, s.MinProperties, s.MaxProperties, s.MinContains, s.MaxContains} {
		if n != nil && *n < 0 {
			return fmt.Errorf("schema limits must be nonnegative")
		}
	}
	if s.MultipleOf != nil && *s.MultipleOf <= 0 {
		return fmt.Errorf("multipleOf must be positive")
	}
	names := map[string]bool{}
	for _, name := range s.Required {
		if names[name] {
			return fmt.Errorf("duplicate required property")
		}
		names[name] = true
	}
	// Traverse only schema-valued fields; user examples/default values are metadata.
	value := reflect.ValueOf(s).Elem()
	schemaType := reflect.TypeOf((*jsonschema.Schema)(nil))
	for i := 0; i < value.NumField(); i++ {
		v := value.Field(i)
		switch v.Kind() {
		case reflect.Pointer:
			if v.Type() == schemaType && !v.IsNil() {
				if e := schemaKeywords(v.Interface().(*jsonschema.Schema), depth+1); e != nil {
					return e
				}
			}
		case reflect.Slice:
			if v.Type().Elem() == schemaType {
				for j := 0; j < v.Len(); j++ {
					if e := schemaKeywords(v.Index(j).Interface().(*jsonschema.Schema), depth+1); e != nil {
						return e
					}
				}
			}
		case reflect.Map:
			if v.Type().Elem() == schemaType {
				for _, key := range v.MapKeys() {
					if e := schemaKeywords(v.MapIndex(key).Interface().(*jsonschema.Schema), depth+1); e != nil {
						return e
					}
				}
			}
		}
	}
	return nil
}
