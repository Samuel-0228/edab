package schema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// ValidatePayload validates raw event payload bytes against a schema.
func ValidatePayload(s *Schema, payload []byte) ([]ValidationError, error) {
	if s == nil {
		return nil, nil // No schema configured, passthrough
	}

	if len(payload) == 0 || string(payload) == "{}" {
		payload = []byte("{}")
	}

	switch s.Type {
	case TypeJSONSchema, "":
		return validateJSONSchema(s.Definition, payload)
	case TypeProtobuf:
		return validateProtobuf(s.Definition, payload)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidSchemaType, s.Type)
	}
}

func validateJSONSchema(definition string, payload []byte) ([]ValidationError, error) {
	var schemaMap map[string]any
	if err := json.Unmarshal([]byte(definition), &schemaMap); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidDefinition, err.Error())
	}

	var data any
	if err := json.Unmarshal(payload, &data); err != nil {
		return []ValidationError{{Field: "$root", Message: "payload is not valid JSON: " + err.Error()}}, nil
	}

	var errs []ValidationError
	validateNode("", schemaMap, data, &errs)
	return errs, nil
}

func validateNode(path string, schema map[string]any, data any, errs *[]ValidationError) {
	expectedType, _ := schema["type"].(string)

	if expectedType != "" {
		if !checkType(expectedType, data) {
			*errs = append(*errs, ValidationError{
				Field:   displayPath(path),
				Message: fmt.Sprintf("expected type %s, got %s", expectedType, reflectType(data)),
				Rule:    "type",
			})
			return
		}
	}

	// Validate Enum
	if enumVals, ok := schema["enum"].([]any); ok && len(enumVals) > 0 {
		matched := false
		for _, ev := range enumVals {
			if fmt.Sprintf("%v", ev) == fmt.Sprintf("%v", data) {
				matched = true
				break
			}
		}
		if !matched {
			*errs = append(*errs, ValidationError{
				Field:   displayPath(path),
				Message: fmt.Sprintf("value %v is not in allowed enum values", data),
				Rule:    "enum",
			})
		}
	}

	// Validate Object Properties & Required Fields
	if dataMap, ok := data.(map[string]any); ok {
		// Required fields
		if reqList, ok := schema["required"].([]any); ok {
			for _, reqKey := range reqList {
				keyStr := fmt.Sprintf("%v", reqKey)
				val, exists := dataMap[keyStr]
				if !exists || val == nil {
					propPath := appendPath(path, keyStr)
					*errs = append(*errs, ValidationError{
						Field:   displayPath(propPath),
						Message: "required property is missing",
						Rule:    "required",
					})
				}
			}
		}

		// Property schema validation
		if props, ok := schema["properties"].(map[string]any); ok {
			for propKey, propSchemaRaw := range props {
				propSchema, isMap := propSchemaRaw.(map[string]any)
				if !isMap {
					continue
				}
				if val, exists := dataMap[propKey]; exists && val != nil {
					validateNode(appendPath(path, propKey), propSchema, val, errs)
				}
			}
		}
	}

	// Validate Array Items
	if dataList, ok := data.([]any); ok {
		if itemSchemaRaw, ok := schema["items"].(map[string]any); ok {
			for i, item := range dataList {
				validateNode(fmt.Sprintf("%s[%d]", path, i), itemSchemaRaw, item, errs)
			}
		}
	}
}

func checkType(expected string, val any) bool {
	if val == nil {
		return expected == "null"
	}
	switch expected {
	case "string":
		_, ok := val.(string)
		return ok
	case "number":
		_, ok := val.(float64)
		return ok
	case "integer":
		f, ok := val.(float64)
		return ok && f == float64(int64(f))
	case "boolean":
		_, ok := val.(bool)
		return ok
	case "object":
		_, ok := val.(map[string]any)
		return ok
	case "array":
		_, ok := val.([]any)
		return ok
	case "null":
		return val == nil
	default:
		return true
	}
}

func reflectType(val any) string {
	if val == nil {
		return "null"
	}
	switch val.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return reflect.TypeOf(val).String()
	}
}

func appendPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func displayPath(path string) string {
	if path == "" {
		return "$root"
	}
	return path
}

func validateProtobuf(definition string, payload []byte) ([]ValidationError, error) {
	// Lightweight schema check for Protobuf definitions (syntax & message declaration)
	if !strings.Contains(definition, "syntax") || !strings.Contains(definition, "message") {
		return nil, fmt.Errorf("%w: missing protobuf syntax or message declaration", ErrInvalidDefinition)
	}

	if len(payload) == 0 {
		return []ValidationError{{Field: "$root", Message: "empty payload for protobuf event"}}, nil
	}

	return nil, nil
}
