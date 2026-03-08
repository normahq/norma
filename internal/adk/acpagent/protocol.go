package acpagent

import "encoding/json"

const (
	protocolVersion = 1

	methodInitialize           = "initialize"
	methodAuthenticate         = "authenticate"
	methodSessionNew           = "session/new"
	methodSessionPrompt        = "session/prompt"
	methodSessionCancel        = "session/cancel"
	methodSessionUpdate        = "session/update"
	methodSessionRequestPermit = "session/request_permission"
)

const (
	updateAgentMessageChunk = "agent_message_chunk"
)

const (
	outcomeCancelled = "cancelled"
	outcomeSelected  = "selected"
)

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type initializeRequest struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities clientCapabilities `json:"clientCapabilities,omitempty"`
	ClientInfo         *implementation    `json:"clientInfo,omitempty"`
}

type clientCapabilities struct {
	FS       *fileSystemCapabilities `json:"fs,omitempty"`
	Terminal bool                    `json:"terminal,omitempty"`
}

type fileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile,omitempty"`
	WriteTextFile bool `json:"writeTextFile,omitempty"`
}

type implementation struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type initializeResponse struct {
	ProtocolVersion int                `json:"protocolVersion"`
	AgentInfo       *implementation    `json:"agentInfo,omitempty"`
	AuthMethods     []authMethod       `json:"authMethods,omitempty"`
	Capabilities    *agentCapabilities `json:"agentCapabilities,omitempty"`
}

type agentCapabilities struct {
	LoadSession bool `json:"loadSession,omitempty"`
}

type authMethod struct {
	ID string `json:"id"`
}

type authenticateRequest struct {
	MethodID string `json:"methodId"`
}

type newSessionRequest struct {
	Cwd        string      `json:"cwd"`
	MCPServers []mcpServer `json:"mcpServers"`
}

type mcpServer struct{}

type newSessionResponse struct {
	SessionID string `json:"sessionId"`
}

type promptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []contentBlock `json:"prompt"`
}

type promptResponse struct {
	StopReason string `json:"stopReason"`
}

type cancelNotification struct {
	SessionID string `json:"sessionId"`
}

type sessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    sessionUpdate `json:"update"`
}

type sessionUpdate struct {
	SessionUpdate string       `json:"sessionUpdate"`
	Content       *textContent `json:"content,omitempty"`
	ToolCallID    string       `json:"toolCallId,omitempty"`
	Title         string       `json:"title,omitempty"`
	Status        string       `json:"status,omitempty"`
	Kind          string       `json:"kind,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type textContent = contentBlock

type requestPermissionRequest struct {
	SessionID string             `json:"sessionId"`
	Options   []permissionOption `json:"options"`
	ToolCall  permissionToolCall `json:"toolCall"`
}

type permissionOption struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	OptionID string `json:"optionId"`
}

type permissionToolCall struct {
	ToolCallID string `json:"toolCallId,omitempty"`
	Title      string `json:"title,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Status     string `json:"status,omitempty"`
}

type requestPermissionResponse struct {
	Outcome permissionOutcome `json:"outcome"`
}

type permissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

type RequestPermissionRequest = requestPermissionRequest

type RequestPermissionResponse = requestPermissionResponse

type PermissionOption = permissionOption

type PermissionOutcome = permissionOutcome
