package contracts

// Role defines the interface for a workflow step implementation.
// Roles are responsible for:
// - Providing their name, input/output schemas.
// - Generating system instructions (prompt).
// - Mapping raw request bytes to role-specific input format.
// - Mapping raw agent output to RawAgentResponse.
type Role interface {
	Name() string
	Schemas() SchemaPair
	Prompt(req RawAgentRequest) (string, error)
	MapRequest(req RawAgentRequest) (any, error)
	MapResponse(outBytes []byte) (RawAgentResponse, error)
}
