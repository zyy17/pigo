package ai

// Property is one field in a tool's JSON schema.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Schema is a small JSON Schema object. The coding tools only need objects.
type Schema struct {
	Type                 string              `json:"type"`
	Properties           map[string]Property `json:"properties,omitempty"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties *bool               `json:"additionalProperties,omitempty"`
}

// Tool is the description sent to the model. It has no execute function.
// Running a tool is the agent's job.
type Tool struct {
	Name        string
	Description string
	Parameters  Schema
}

// StringProperty describes a string parameter.
func StringProperty(description string) Property {
	return Property{Type: "string", Description: description}
}

// BoolProperty describes a boolean parameter.
func BoolProperty(description string) Property {
	return Property{Type: "boolean", Description: description}
}

// ObjectSchema builds an object schema that rejects unknown fields.
func ObjectSchema(properties map[string]Property, required ...string) Schema {
	closed := false
	return Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &closed,
	}
}
