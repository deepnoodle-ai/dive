package schema

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Generate generates a JSON schema for the given Go type.
// It uses reflection to analyze the type structure and returns a Schema
// that can be used for JSON validation or LLM response formatting.
//
// Example:
//
//	type User struct {
//	  Name  string `json:"name" description:"User's full name"`
//	  Age   int    `json:"age,omitempty" description:"User's age"`
//	  Email string `json:"email" required:"true"`
//	}
//	schema, err := Generate(User{})
func Generate(v any) (*Schema, error) {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil, fmt.Errorf("cannot generate schema for nil value")
	}

	// For non-struct types, we create a simple schema
	if t.Kind() != reflect.Struct && (t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct) {
		prop, err := reflectType(t)
		if err != nil {
			return nil, err
		}
		return &Schema{
			Type: prop.Type,
		}, nil
	}

	// For struct types, generate a full object schema
	prop, err := reflectType(t)
	if err != nil {
		return nil, err
	}

	additionalProps := false
	return &Schema{
		Type:                 prop.Type,
		Properties:           prop.Properties,
		Required:             prop.Required,
		AdditionalProperties: &additionalProps,
	}, nil
}

// reflectType recursively analyzes a reflect.Type and returns a Property
// that describes its JSON schema representation.
func reflectType(t reflect.Type) (*Property, error) {
	switch t.Kind() {
	case reflect.String:
		return &Property{Type: String}, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Property{Type: Integer}, nil

	case reflect.Float32, reflect.Float64:
		return &Property{Type: Number}, nil

	case reflect.Bool:
		return &Property{Type: Boolean}, nil

	case reflect.Slice, reflect.Array:
		items, err := reflectType(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("failed to reflect array/slice element type: %w", err)
		}
		return &Property{
			Type:  Array,
			Items: items,
		}, nil

	case reflect.Struct:
		return reflectStruct(t)

	case reflect.Ptr:
		// For pointer types, we reflect the underlying type but mark it as nullable
		underlying, err := reflectType(t.Elem())
		if err != nil {
			return nil, fmt.Errorf("failed to reflect pointer underlying type: %w", err)
		}
		nullable := true
		underlying.Nullable = &nullable
		return underlying, nil

	case reflect.Interface:
		// For interface{} or any, we don't specify a type to allow any JSON value
		return &Property{}, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", t.Kind().String())
	}
}

// reflectStruct analyzes a struct type and returns a Property representing
// an object schema with properties, required fields, and other constraints.
func reflectStruct(t reflect.Type) (*Property, error) {
	properties := make(map[string]*Property)
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Parse JSON tag to get field name and options
		jsonName, isRequired := parseJSONTag(field)
		if jsonName == "-" {
			// Skip fields marked with json:"-"
			continue
		}

		// Generate property schema for the field type
		prop, err := reflectType(field.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to reflect field %s: %w", field.Name, err)
		}

		// Apply field tags to the property
		applyFieldTags(prop, field)

		// Check if field is required (from tag or default behavior)
		if checkRequired(field, isRequired) {
			required = append(required, jsonName)
		}

		properties[jsonName] = prop
	}

	additionalProps := false
	return &Property{
		Type:                 Object,
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &additionalProps,
	}, nil
}

// parseJSONTag extracts the JSON field name and omitempty flag from a struct field's json tag.
// Returns the field name and whether the field is required (not omitempty).
func parseJSONTag(field reflect.StructField) (name string, required bool) {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" {
		return field.Name, true
	}

	parts := strings.Split(jsonTag, ",")
	name = parts[0]
	if name == "" {
		name = field.Name
	}

	// Check for omitempty flag
	required = true
	for _, part := range parts[1:] {
		if part == "omitempty" {
			required = false
			break
		}
	}

	return name, required
}

// applyFieldTags applies various struct field tags to a Property.
func applyFieldTags(prop *Property, field reflect.StructField) {
	// Description tag
	if desc := field.Tag.Get("description"); desc != "" {
		prop.Description = desc
	}

	// Enum tag (comma-separated values)
	if enum := field.Tag.Get("enum"); enum != "" {
		prop.Enum = strings.Split(enum, ",")
	}

	// Nullable tag
	if nullable := field.Tag.Get("nullable"); nullable != "" {
		if val, err := strconv.ParseBool(nullable); err == nil {
			prop.Nullable = &val
		}
	}

	// Pattern tag (for string validation)
	if pattern := field.Tag.Get("pattern"); pattern != "" {
		prop.Pattern = &pattern
	}

	// Format tag (e.g., "email", "date-time")
	if format := field.Tag.Get("format"); format != "" {
		prop.Format = &format
	}

	// Min/max length for strings
	if minLen := field.Tag.Get("minLength"); minLen != "" {
		if val, err := strconv.Atoi(minLen); err == nil {
			prop.MinLength = &val
		}
	}
	if maxLen := field.Tag.Get("maxLength"); maxLen != "" {
		if val, err := strconv.Atoi(maxLen); err == nil {
			prop.MaxLength = &val
		}
	}

	// Min/max for numbers
	if min := field.Tag.Get("minimum"); min != "" {
		if val, err := strconv.ParseFloat(min, 64); err == nil {
			prop.Minimum = &val
		}
	}
	if max := field.Tag.Get("maximum"); max != "" {
		if val, err := strconv.ParseFloat(max, 64); err == nil {
			prop.Maximum = &val
		}
	}

	// Min/max items for arrays
	if minItems := field.Tag.Get("minItems"); minItems != "" {
		if val, err := strconv.Atoi(minItems); err == nil {
			prop.MinItems = &val
		}
	}
	if maxItems := field.Tag.Get("maxItems"); maxItems != "" {
		if val, err := strconv.Atoi(maxItems); err == nil {
			prop.MaxItems = &val
		}
	}
}

// checkRequired determines if a field should be marked as required.
// It considers both the JSON tag omitempty flag and an explicit required tag.
func checkRequired(field reflect.StructField, jsonRequired bool) bool {
	// Explicit required tag takes precedence
	if req := field.Tag.Get("required"); req != "" {
		if val, err := strconv.ParseBool(req); err == nil {
			return val
		}
	}

	// Otherwise, use the result from JSON tag parsing
	return jsonRequired
}
