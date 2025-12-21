package dive

import "github.com/deepnoodle-ai/wonton/schema"

// Type aliases for easy access to schema types used in tool definitions
type (
	Schema         = schema.Schema
	SchemaProperty = schema.Property
	SchemaType     = schema.SchemaType
)

// SchemaType constants for JSON Schema types
const (
	Object  SchemaType = schema.Object
	Array   SchemaType = schema.Array
	String  SchemaType = schema.String
	Integer SchemaType = schema.Integer
	Number  SchemaType = schema.Number
	Boolean SchemaType = schema.Boolean
	Null    SchemaType = schema.Null
)

// NewSchema creates a new Schema with the given properties and required fields.
var NewSchema = schema.NewSchema
