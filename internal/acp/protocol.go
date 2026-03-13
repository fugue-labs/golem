// Package acp implements the Agent Client Protocol (ACP), enabling golem
// to serve as an agent backend for any ACP-compatible editor (Zed, Neovim,
// JetBrains, etc.) over JSON-RPC 2.0 stdio transport.
package acp

import "encoding/json"

// Protocol version. Incremented only for breaking changes.
const ProtocolVersion = 1

// --- JSON-RPC 2.0 envelope types ---

// Request is an incoming JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is an outgoing JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Notification is an outgoing JSON-RPC 2.0 notification (no id).
type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// RPCError is a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParse          = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal       = -32603
	ErrCodeServerError    = -32000
	ErrCodeSessionUnknown = -32002
)

// --- Implementation metadata ---

// Implementation identifies a client or agent.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}

// --- Capabilities ---

// ClientCapabilities describes what the client supports.
type ClientCapabilities struct {
	FileSystem *FileSystemCapabilities `json:"fs,omitempty"`
	Terminal   bool                    `json:"terminal,omitempty"`
}

// FileSystemCapabilities describes client file system support.
type FileSystemCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

// AgentCapabilities describes what the agent supports.
type AgentCapabilities struct {
	Prompt      *PromptCapabilities `json:"prompt,omitempty"`
	Session     bool                `json:"session,omitempty"`
	LoadSession bool                `json:"loadSession,omitempty"`
	Mcp         *McpCapabilities    `json:"mcp,omitempty"`
}

// PromptCapabilities describes supported prompt content types.
type PromptCapabilities struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// McpCapabilities describes supported MCP transports.
type McpCapabilities struct {
	HTTP bool `json:"http,omitempty"`
	SSE  bool `json:"sse,omitempty"`
}

// --- initialize ---

// InitializeParams are the parameters for the "initialize" request.
type InitializeParams struct {
	ProtocolVersion    int                 `json:"protocolVersion"`
	ClientInfo         *Implementation     `json:"clientInfo,omitempty"`
	ClientCapabilities *ClientCapabilities `json:"clientCapabilities,omitempty"`
}

// InitializeResult is the response to "initialize".
type InitializeResult struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentInfo         Implementation    `json:"agentInfo"`
	AgentCapabilities AgentCapabilities `json:"agentCapabilities"`
}

// --- session/new ---

// NewSessionParams are the parameters for "session/new".
type NewSessionParams struct {
	Cwd        string          `json:"cwd"`
	McpServers json.RawMessage `json:"mcpServers,omitempty"` // stored but not parsed
}

// NewSessionResult is the response to "session/new".
type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

// --- session/load ---

// LoadSessionParams are the parameters for "session/load".
type LoadSessionParams struct {
	SessionID  string          `json:"sessionId"`
	Cwd        string          `json:"cwd"`
	McpServers json.RawMessage `json:"mcpServers,omitempty"`
}

// LoadSessionResult is the response to "session/load".
type LoadSessionResult struct{}

// --- session/list ---

// ListSessionsParams are the parameters for "session/list".
type ListSessionsParams struct {
	Cwd    string `json:"cwd,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// SessionInfo describes a session in a list response.
type SessionInfo struct {
	SessionID string `json:"sessionId"`
	Title     string `json:"title,omitempty"`
}

// ListSessionsResult is the response to "session/list".
type ListSessionsResult struct {
	Sessions   []SessionInfo `json:"sessions"`
	NextCursor string        `json:"nextCursor,omitempty"`
}

// --- session/prompt ---

// PromptParams are the parameters for "session/prompt".
type PromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// PromptResult is the response to "session/prompt".
type PromptResult struct {
	StopReason StopReason `json:"stopReason"`
}

// StopReason describes why a prompt turn ended.
type StopReason string

const (
	StopReasonEndTurn         StopReason = "end_turn"
	StopReasonMaxTokens       StopReason = "max_tokens"
	StopReasonMaxTurnRequests StopReason = "max_turn_requests"
	StopReasonRefusal         StopReason = "refusal"
	StopReasonCancelled       StopReason = "cancelled"
)

// --- session/cancel ---

// CancelParams are the parameters for the "session/cancel" notification.
type CancelParams struct {
	SessionID string `json:"sessionId"`
}

// --- Content blocks ---

// ContentBlock is a single unit of content in a prompt or response.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}

// --- session/update notification ---

// SessionUpdateParams wraps the streaming update notification.
type SessionUpdateParams struct {
	SessionID string       `json:"sessionId"`
	Update    SessionUpdate `json:"update"`
}

// SessionUpdate is a discriminated union of streaming updates.
// The SessionUpdate field acts as the discriminator.
type SessionUpdate struct {
	// Discriminator: "agent_message_chunk", "tool_call", "tool_call_update", "plan"
	SessionUpdate string `json:"sessionUpdate"`

	// agent_message_chunk fields
	Content *ContentBlock `json:"content,omitempty"`

	// tool_call / tool_call_update shared fields
	ToolCallID string          `json:"toolCallId,omitempty"`
	Title      string          `json:"title,omitempty"`
	Kind       ToolKind        `json:"kind,omitempty"`
	Status     ToolCallStatus  `json:"status,omitempty"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage `json:"rawOutput,omitempty"`

	// tool_call_update content (output blocks)
	ToolContent []ToolCallContent `json:"content,omitempty"` // renamed to avoid conflict; see MarshalJSON

	// plan fields
	Entries []PlanEntry `json:"entries,omitempty"`
}

// MarshalJSON handles the union serialization, producing only relevant fields
// for each session update type.
func (u SessionUpdate) MarshalJSON() ([]byte, error) {
	switch u.SessionUpdate {
	case "agent_message_chunk":
		return json.Marshal(struct {
			SessionUpdate string       `json:"sessionUpdate"`
			Content       *ContentBlock `json:"content"`
		}{u.SessionUpdate, u.Content})

	case "tool_call":
		type tc struct {
			SessionUpdate string          `json:"sessionUpdate"`
			ToolCallID    string          `json:"toolCallId"`
			Title         string          `json:"title"`
			Kind          ToolKind        `json:"kind"`
			Status        ToolCallStatus  `json:"status"`
			RawInput      json.RawMessage `json:"rawInput,omitempty"`
		}
		return json.Marshal(tc{u.SessionUpdate, u.ToolCallID, u.Title, u.Kind, u.Status, u.RawInput})

	case "tool_call_update":
		type tcu struct {
			SessionUpdate string            `json:"sessionUpdate"`
			ToolCallID    string            `json:"toolCallId"`
			Status        ToolCallStatus    `json:"status"`
			Content       []ToolCallContent `json:"content,omitempty"`
		}
		return json.Marshal(tcu{u.SessionUpdate, u.ToolCallID, u.Status, u.ToolContent})

	case "plan":
		return json.Marshal(struct {
			SessionUpdate string      `json:"sessionUpdate"`
			Entries       []PlanEntry `json:"entries"`
		}{u.SessionUpdate, u.Entries})

	default:
		// Fallback: marshal all fields.
		type Alias SessionUpdate
		return json.Marshal(Alias(u))
	}
}

// ToolKind categorizes tool operations.
type ToolKind string

const (
	ToolKindRead    ToolKind = "read"
	ToolKindEdit    ToolKind = "edit"
	ToolKindDelete  ToolKind = "delete"
	ToolKindMove    ToolKind = "move"
	ToolKindSearch  ToolKind = "search"
	ToolKindExecute ToolKind = "execute"
	ToolKindThink   ToolKind = "think"
	ToolKindFetch   ToolKind = "fetch"
	ToolKindOther   ToolKind = "other"
)

// ToolCallStatus tracks a tool call's lifecycle.
type ToolCallStatus string

const (
	ToolCallPending    ToolCallStatus = "pending"
	ToolCallInProgress ToolCallStatus = "in_progress"
	ToolCallCompleted  ToolCallStatus = "completed"
	ToolCallFailed     ToolCallStatus = "failed"
	ToolCallCancelled  ToolCallStatus = "cancelled"
)

// ToolCallContent wraps content produced by a tool call.
type ToolCallContent struct {
	Type    string       `json:"type"` // "content" or "diff" or "terminal"
	Content *ContentBlock `json:"content,omitempty"`
}

// PlanEntry represents a task in the agent's execution plan.
type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"` // high, medium, low
	Status   string `json:"status,omitempty"`   // pending, in_progress, completed
}

// --- session/request_permission ---

// RequestPermissionParams are sent from agent to client for tool approval.
type RequestPermissionParams struct {
	SessionID string           `json:"sessionId"`
	ToolCall  ToolCallForPerm  `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

// ToolCallForPerm is a simplified tool call reference for permission requests.
type ToolCallForPerm struct {
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title"`
	Kind       ToolKind        `json:"kind"`
	RawInput   json.RawMessage `json:"rawInput,omitempty"`
}

// PermissionOption represents a user-selectable authorization choice.
type PermissionOption struct {
	OptionID string             `json:"optionId"`
	Name     string             `json:"name"`
	Kind     PermissionOptionKind `json:"kind"`
}

// PermissionOptionKind categorizes the permission choice.
type PermissionOptionKind string

const (
	PermissionAllowOnce    PermissionOptionKind = "allow_once"
	PermissionAllowAlways  PermissionOptionKind = "allow_always"
	PermissionRejectOnce   PermissionOptionKind = "reject_once"
	PermissionRejectAlways PermissionOptionKind = "reject_always"
)

// RequestPermissionResult is the client's response to a permission request.
type RequestPermissionResult struct {
	Outcome  string `json:"outcome"` // "cancelled" or "selected"
	Selected *struct {
		OptionID string `json:"optionId"`
	} `json:"selected,omitempty"`
}
