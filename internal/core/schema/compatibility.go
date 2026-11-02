package schema

import (
	"encoding/json"
	"fmt"
)

// CheckCompatibility checks if newSchema is compatible with oldSchema
// according to the configured CompatibilityMode.
func CheckCompatibility(oldSchema, newSchema *Schema) error {
	if oldSchema == nil || newSchema == nil {
		return nil
	}

	mode := newSchema.Compatibility
	if mode == "" {
		mode = oldSchema.Compatibility
	}
	if mode == "" {
		mode = CompatibilityBackward
	}

	if mode == CompatibilityNone {
		return nil
	}

	oldType := oldSchema.Type
	if oldType == "" {
		oldType = TypeJSONSchema
	}
	newType := newSchema.Type
	if newType == "" {
		newType = TypeJSONSchema
	}
	if oldType != newType {
		return fmt.Errorf("%w: cannot change schema type from %s to %s",
			ErrIncompatibleSchema, oldType, newType)
	}

	if oldSchema.Type == TypeJSONSchema || oldSchema.Type == "" {
		return checkJSONSchemaCompatibility(oldSchema.Definition, newSchema.Definition, mode)
	}

	return nil
}

func checkJSONSchemaCompatibility(oldDef, newDef string, mode CompatibilityMode) error {
	var oldMap, newMap map[string]any
	if err := json.Unmarshal([]byte(oldDef), &oldMap); err != nil {
		return fmt.Errorf("invalid existing schema definition: %w", err)
	}
	if err := json.Unmarshal([]byte(newDef), &newMap); err != nil {
		return fmt.Errorf("invalid new schema definition: %w", err)
	}

	oldRequired := extractRequired(oldMap)
	newRequired := extractRequired(newMap)

	oldProps := extractProperties(oldMap)
	newProps := extractProperties(newMap)

	// BACKWARD Compatibility check:
	// New schema must not add new required fields (existing data would fail validation)
	if mode == CompatibilityBackward || mode == CompatibilityFull {
		for reqField := range newRequired {
			if !oldRequired[reqField] {
				// Field is required in new schema but was not required in old schema
				if _, existedInOld := oldProps[reqField]; !existedInOld {
					return fmt.Errorf("%w (BACKWARD): new schema adds required field %q which does not exist in old schema",
						ErrIncompatibleSchema, reqField)
				}
			}
		}

		// Property types in old schema must not be narrowed or changed incompatibly
		for propName, oldProp := range oldProps {
			if newProp, exists := newProps[propName]; exists {
				oldType := getPropertyType(oldProp)
				newType := getPropertyType(newProp)
				if oldType != "" && newType != "" && oldType != newType {
					return fmt.Errorf("%w (BACKWARD): property %q type changed from %s to %s",
						ErrIncompatibleSchema, propName, oldType, newType)
				}
			}
		}
	}

	// FORWARD Compatibility check:
	// Old schema consumers must accept data from new schema.
	// New schema must not delete fields that old schema marked as required.
	if mode == CompatibilityForward || mode == CompatibilityFull {
		for oldReq := range oldRequired {
			if _, exists := newProps[oldReq]; !exists {
				return fmt.Errorf("%w (FORWARD): new schema removes required field %q present in old schema",
					ErrIncompatibleSchema, oldReq)
			}
		}
	}

	return nil
}

func extractRequired(schema map[string]any) map[string]bool {
	res := make(map[string]bool)
	if reqList, ok := schema["required"].([]any); ok {
		for _, r := range reqList {
			res[fmt.Sprintf("%v", r)] = true
		}
	}
	return res
}

func extractProperties(schema map[string]any) map[string]map[string]any {
	res := make(map[string]map[string]any)
	if props, ok := schema["properties"].(map[string]any); ok {
		for k, v := range props {
			if m, ok := v.(map[string]any); ok {
				res[k] = m
			}
		}
	}
	return res
}

func getPropertyType(prop map[string]any) string {
	t, _ := prop["type"].(string)
	return t
}
