package sessionmcp

type noInput struct{}

// ToolError represents an error from a tool operation.
type ToolError struct {
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// ToolOutcome represents the result of a tool operation.
type ToolOutcome struct {
	OK    bool       `json:"ok"`
	Error *ToolError `json:"error,omitempty"`
}

type basicOutput struct {
	ToolOutcome
}

// Get input/output

type getKeyInput struct {
	Key string `json:"key" jsonschema:"state key to retrieve"`
}

type getKeyOutput struct {
	ToolOutcome
	Value string `json:"value,omitempty"`
	Found bool   `json:"found"`
}

// Set input/output

type setKeyInput struct {
	Key   string `json:"key" jsonschema:"state key"`
	Value string `json:"value" jsonschema:"state value (JSON string)"`
}

// Delete input/output

type deleteKeyInput struct {
	Key string `json:"key" jsonschema:"state key to delete"`
}

// List input/output

type listKeysInput struct {
	Prefix string `json:"prefix,omitempty" jsonschema:"optional key prefix filter"`
}

type listKeysOutput struct {
	ToolOutcome
	Keys []string `json:"keys,omitempty"`
}

// GetJSON input/output - returns parsed JSON value

type getJSONInput struct {
	Key string `json:"key" jsonschema:"state key to retrieve as JSON"`
}

type getJSONOutput struct {
	ToolOutcome
	Value interface{} `json:"value,omitempty"`
	Found bool        `json:"found"`
}

// SetJSON input/output - stores a value as JSON

type setJSONInput struct {
	Key   string      `json:"key" jsonschema:"state key"`
	Value interface{} `json:"value" jsonschema:"value to store as JSON"`
}

// MergeJSON input/output - merges JSON object into existing value

type mergeJSONInput struct {
	Key   string                 `json:"key" jsonschema:"state key (must contain JSON object)"`
	Value map[string]interface{} `json:"value" jsonschema:"fields to merge into existing object"`
}

type mergeJSONOutput struct {
	ToolOutcome
	Merged map[string]interface{} `json:"merged,omitempty"`
}

// Keyspace input/output - for scoping keys to a session/agent

type keyspaceInput struct {
	Namespace string `json:"namespace" jsonschema:"namespace for key isolation (e.g., session-id or agent-name)"`
	Key       string `json:"key" jsonschema:"key within namespace"`
}

type keyspaceValueInput struct {
	Namespace string `json:"namespace" jsonschema:"namespace for key isolation"`
	Key       string `json:"key" jsonschema:"key within namespace"`
	Value     string `json:"value" jsonschema:"value to store"`
}

type keyspaceJSONInput struct {
	Namespace string      `json:"namespace" jsonschema:"namespace for key isolation"`
	Key       string      `json:"key" jsonschema:"key within namespace"`
	Value     interface{} `json:"value" jsonschema:"value to store as JSON"`
}

type namespaceOnlyInput struct {
	Namespace string `json:"namespace" jsonschema:"namespace to list keys from"`
}
