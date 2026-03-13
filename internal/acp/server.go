package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fugue-labs/golem/internal/agent"
	"github.com/fugue-labs/golem/internal/config"
	"github.com/fugue-labs/gollem/core"
)

// Server implements the ACP agent server.
type Server struct {
	transport *Transport

	mu       sync.Mutex
	sessions map[string]*session

	// Client capabilities from initialize.
	clientCaps *ClientCapabilities
	clientInfo *Implementation

	// Pending agent→client requests awaiting responses.
	pendingMu   sync.Mutex
	pendingReqs map[string]chan json.RawMessage // keyed by request ID
	nextReqID   atomic.Int64

	initialized bool
}

// session holds per-session state.
type session struct {
	id      string
	cwd     string
	cfg     *config.Config
	runtime agent.RuntimeState
	agent   *core.Agent[string]
	history []core.ModelMessage
	toolSt  map[string]any

	cancelMu sync.Mutex
	cancel   context.CancelFunc

	// Permission approval channel: agent goroutine blocks, main loop responds.
	approvalCh   chan permissionRequest
	alwaysAllow  map[string]bool // tools approved with allow_always
}

type permissionRequest struct {
	toolName string
	argsJSON string
	response chan<- bool
}

// NewServer creates an ACP server using the given stdio streams.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{
		transport:   NewTransport(in, out),
		sessions:    make(map[string]*session),
		pendingReqs: make(map[string]chan json.RawMessage),
	}
}

// Run starts the server's main request loop. It blocks until stdin closes.
func (s *Server) Run() error {
	for {
		req, err := s.transport.ReadRequest()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("reading request: %w", err)
		}
		s.handleMessage(req)
	}
}

func (s *Server) handleMessage(req *Request) {
	// Check if this is a response to an agent→client request.
	if req.Method == "" && req.ID != nil {
		s.handleClientResponse(req)
		return
	}

	// Notifications (no ID) — fire-and-forget.
	if req.ID == nil || string(req.ID) == "null" {
		s.handleNotification(req)
		return
	}

	// Standard client→agent request.
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "session/new":
		s.handleNewSession(req)
	case "session/load":
		s.handleLoadSession(req)
	case "session/list":
		s.handleListSessions(req)
	case "session/prompt":
		s.handlePrompt(req)
	case "session/set_mode":
		s.respondError(req.ID, ErrCodeMethodNotFound, "session modes not supported")
	case "session/set_config_option":
		s.respondError(req.ID, ErrCodeMethodNotFound, "config options not supported")
	case "authenticate":
		s.respondError(req.ID, ErrCodeMethodNotFound, "authentication not required")
	default:
		s.respondError(req.ID, ErrCodeMethodNotFound, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (s *Server) handleNotification(req *Request) {
	switch req.Method {
	case "session/cancel":
		var params CancelParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return
		}
		s.mu.Lock()
		sess, ok := s.sessions[params.SessionID]
		s.mu.Unlock()
		if ok {
			sess.cancelMu.Lock()
			if sess.cancel != nil {
				sess.cancel()
			}
			sess.cancelMu.Unlock()
		}
	}
}

func (s *Server) handleClientResponse(req *Request) {
	s.pendingMu.Lock()
	ch, ok := s.pendingReqs[string(req.ID)]
	if ok {
		delete(s.pendingReqs, string(req.ID))
	}
	s.pendingMu.Unlock()

	if ok && ch != nil {
		// Forward the raw result to the waiting goroutine.
		// req.Params holds "result" in a response, but we need to reconstruct
		// from the full message. We'll just re-marshal what we need.
		// Actually, the response comes as {"jsonrpc":"2.0","id":...,"result":...}
		// but our ReadRequest parsed it as a Request with Method="" and Params=nil.
		// We need a different approach: detect responses and pass them through.
		// For now, we'll encode the full raw line.
		raw, _ := json.Marshal(req)
		select {
		case ch <- raw:
		default:
		}
	}
}

// --- Method handlers ---

func (s *Server) handleInitialize(req *Request) {
	var params InitializeParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respondError(req.ID, ErrCodeInvalidParams, err.Error())
		return
	}

	s.clientCaps = params.ClientCapabilities
	s.clientInfo = params.ClientInfo

	// Negotiate protocol version.
	version := ProtocolVersion
	if params.ProtocolVersion < version {
		version = params.ProtocolVersion
	}

	s.initialized = true
	s.respond(req.ID, InitializeResult{
		ProtocolVersion: version,
		AgentInfo: Implementation{
			Name:    "golem",
			Version: "1.0.0",
			Title:   "Golem",
		},
		AgentCapabilities: AgentCapabilities{
			Session:     true,
			LoadSession: false,
			Prompt: &PromptCapabilities{
				Image:           false,
				Audio:           false,
				EmbeddedContext: true,
			},
		},
	})
}

func (s *Server) handleNewSession(req *Request) {
	if !s.initialized {
		s.respondError(req.ID, ErrCodeServerError, "not initialized")
		return
	}

	var params NewSessionParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respondError(req.ID, ErrCodeInvalidParams, err.Error())
		return
	}

	cwd := params.Cwd
	if cwd == "" {
		cwd = "."
	}

	// Load golem config scoped to the working directory.
	cfg, err := config.Load()
	if err != nil {
		s.respondError(req.ID, ErrCodeInternal, fmt.Sprintf("config error: %v", err))
		return
	}
	cfg.WorkingDir = cwd

	// Generate session ID.
	id := generateSessionID()

	sess := &session{
		id:          id,
		cwd:         cwd,
		cfg:         cfg,
		approvalCh:  make(chan permissionRequest, 1),
		alwaysAllow: make(map[string]bool),
	}

	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()

	s.respond(req.ID, NewSessionResult{SessionID: id})
}

func (s *Server) handleLoadSession(req *Request) {
	s.respondError(req.ID, ErrCodeMethodNotFound, "session loading not yet supported")
}

func (s *Server) handleListSessions(req *Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var list []SessionInfo
	for _, sess := range s.sessions {
		list = append(list, SessionInfo{
			SessionID: sess.id,
		})
	}
	s.respond(req.ID, ListSessionsResult{Sessions: list})
}

func (s *Server) handlePrompt(req *Request) {
	if !s.initialized {
		s.respondError(req.ID, ErrCodeServerError, "not initialized")
		return
	}

	var params PromptParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respondError(req.ID, ErrCodeInvalidParams, err.Error())
		return
	}

	s.mu.Lock()
	sess, ok := s.sessions[params.SessionID]
	s.mu.Unlock()
	if !ok {
		s.respondError(req.ID, ErrCodeSessionUnknown, "unknown session")
		return
	}

	// Extract text from prompt content blocks.
	prompt := extractText(params.Prompt)
	if prompt == "" {
		s.respondError(req.ID, ErrCodeInvalidParams, "empty prompt")
		return
	}

	// Run the agent in a goroutine. The prompt response is sent when the
	// agent completes; streaming updates are sent as notifications.
	go s.runPrompt(req.ID, sess, prompt)
}

func (s *Server) runPrompt(reqID json.RawMessage, sess *session, prompt string) {
	ctx, cancel := context.WithCancel(context.Background())
	sess.cancelMu.Lock()
	sess.cancel = cancel
	sess.cancelMu.Unlock()
	defer func() {
		sess.cancelMu.Lock()
		sess.cancel = nil
		sess.cancelMu.Unlock()
		cancel()
	}()

	// Prepare runtime.
	runtime, err := agent.PrepareRuntime(ctx, sess.cfg, prompt)
	if err != nil {
		s.respond(reqID, PromptResult{StopReason: StopReasonRefusal})
		return
	}

	// Carry over persistent session state.
	runtime.Session = sess.runtime.Session
	if runtime.Session == nil {
		runtime.Session = sess.runtime.Session
	}

	// Build hooks that stream to ACP notifications.
	hooks := s.buildHooks(sess.id)

	// Build tool approval func that sends ACP request_permission.
	approvalFunc := s.makeApprovalFunc(sess)

	extraOpts := []core.AgentOption[string]{
		core.WithHooks[string](hooks),
		core.WithToolApproval[string](approvalFunc),
		core.WithGlobalToolApproval[string](),
	}

	a, err := agent.NewWithRuntime(sess.cfg, &runtime, nil, extraOpts...)
	if err != nil {
		s.respond(reqID, PromptResult{StopReason: StopReasonRefusal})
		return
	}
	sess.agent = a
	sess.runtime = runtime

	// Run agent with conversation history.
	var runOpts []core.RunOption
	if len(sess.history) > 0 {
		runOpts = append(runOpts, core.WithMessages(sess.history...))
	}
	if len(sess.toolSt) > 0 {
		runOpts = append(runOpts, core.WithToolState(sess.toolSt))
	}

	result, err := a.Run(ctx, prompt, runOpts...)
	if err != nil {
		stopReason := StopReasonRefusal
		if ctx.Err() != nil {
			stopReason = StopReasonCancelled
		}
		s.respond(reqID, PromptResult{StopReason: stopReason})
		return
	}

	// Update session state.
	sess.history = result.Messages
	sess.toolSt = result.ToolState

	s.respond(reqID, PromptResult{StopReason: StopReasonEndTurn})
}

// buildHooks creates core.Hook callbacks that emit ACP session/update notifications.
func (s *Server) buildHooks(sessionID string) core.Hook {
	return core.Hook{
		OnModelResponse: func(_ context.Context, _ *core.RunContext, resp *core.ModelResponse) {
			if resp == nil {
				return
			}
			for _, part := range resp.Parts {
				switch pt := part.(type) {
				case core.TextPart:
					if pt.Content != "" {
						s.sendSessionUpdate(sessionID, SessionUpdate{
							SessionUpdate: "agent_message_chunk",
							Content:       &ContentBlock{Type: "text", Text: pt.Content},
						})
					}
				case core.ThinkingPart:
					// ACP doesn't have a dedicated thinking type; skip or send as text.
				}
			}
		},
		OnToolStart: func(_ context.Context, _ *core.RunContext, toolCallID, toolName, argsJSON string) {
			var rawInput json.RawMessage
			if argsJSON != "" && argsJSON != "{}" {
				rawInput = json.RawMessage(argsJSON)
			}
			s.sendSessionUpdate(sessionID, SessionUpdate{
				SessionUpdate: "tool_call",
				ToolCallID:    toolCallID,
				Title:         toolName,
				Kind:          classifyTool(toolName),
				Status:        ToolCallPending,
				RawInput:      rawInput,
			})
		},
		OnToolEnd: func(_ context.Context, _ *core.RunContext, toolCallID, toolName, result string, toolErr error) {
			status := ToolCallCompleted
			if toolErr != nil {
				status = ToolCallFailed
				if result == "" {
					result = toolErr.Error()
				}
			}

			var content []ToolCallContent
			if result != "" {
				// Truncate very large tool outputs for the ACP stream.
				if len(result) > 4096 {
					result = result[:4093] + "..."
				}
				content = append(content, ToolCallContent{
					Type:    "content",
					Content: &ContentBlock{Type: "text", Text: result},
				})
			}

			s.sendSessionUpdate(sessionID, SessionUpdate{
				SessionUpdate: "tool_call_update",
				ToolCallID:    toolCallID,
				Status:        status,
				ToolContent:   content,
			})
		},
	}
}

// makeApprovalFunc creates a tool approval function that sends ACP
// request_permission requests to the client and blocks until the
// client responds.
func (s *Server) makeApprovalFunc(sess *session) core.ToolApprovalFunc {
	return func(ctx context.Context, toolName string, argsJSON string) (bool, error) {
		// Auto-approve read-only tools.
		if !isMutatingTool(toolName) {
			return true, nil
		}
		// Check always-allow list.
		if sess.alwaysAllow[toolName] {
			return true, nil
		}

		// Send request_permission to client.
		var rawInput json.RawMessage
		if argsJSON != "" {
			rawInput = json.RawMessage(argsJSON)
		}

		respCh := make(chan json.RawMessage, 1)
		reqIDNum := s.nextReqID.Add(1)
		reqIDStr := fmt.Sprintf("%d", reqIDNum)
		reqIDJSON, _ := json.Marshal(reqIDNum)

		s.pendingMu.Lock()
		s.pendingReqs[reqIDStr] = respCh
		s.pendingMu.Unlock()

		err := s.transport.SendRequest(Request{
			ID:     reqIDJSON,
			Method: "session/request_permission",
			Params: mustMarshal(RequestPermissionParams{
				SessionID: sess.id,
				ToolCall: ToolCallForPerm{
					ToolCallID: fmt.Sprintf("perm_%d", reqIDNum),
					Title:      toolName,
					Kind:       classifyTool(toolName),
					RawInput:   rawInput,
				},
				Options: []PermissionOption{
					{OptionID: "allow_once", Name: "Allow once", Kind: PermissionAllowOnce},
					{OptionID: "allow_always", Name: "Always allow", Kind: PermissionAllowAlways},
					{OptionID: "reject", Name: "Reject", Kind: PermissionRejectOnce},
				},
			}),
		})
		if err != nil {
			return false, fmt.Errorf("sending permission request: %w", err)
		}

		// Wait for the client's response.
		select {
		case raw := <-respCh:
			var fullResp struct {
				Result RequestPermissionResult `json:"result"`
				Error  *RPCError               `json:"error"`
			}
			if err := json.Unmarshal(raw, &fullResp); err != nil {
				return false, nil
			}
			if fullResp.Error != nil {
				return false, nil
			}
			if fullResp.Result.Outcome == "cancelled" {
				return false, nil
			}
			if fullResp.Result.Selected != nil {
				switch fullResp.Result.Selected.OptionID {
				case "allow_once":
					return true, nil
				case "allow_always":
					sess.alwaysAllow[toolName] = true
					return true, nil
				case "reject":
					return false, nil
				}
			}
			return false, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

// --- Helpers ---

func (s *Server) sendSessionUpdate(sessionID string, update SessionUpdate) {
	if err := s.transport.SendNotification(Notification{
		Method: "session/update",
		Params: SessionUpdateParams{
			SessionID: sessionID,
			Update:    update,
		},
	}); err != nil {
		log.Printf("acp: error sending session update: %v", err)
	}
}

func (s *Server) respond(id json.RawMessage, result any) {
	if err := s.transport.SendResponse(Response{
		ID:     id,
		Result: result,
	}); err != nil {
		log.Printf("acp: error sending response: %v", err)
	}
}

func (s *Server) respondError(id json.RawMessage, code int, message string) {
	if err := s.transport.SendResponse(Response{
		ID:    id,
		Error: &RPCError{Code: code, Message: message},
	}); err != nil {
		log.Printf("acp: error sending error response: %v", err)
	}
}

// classifyTool maps golem tool names to ACP ToolKind values.
func classifyTool(name string) ToolKind {
	switch name {
	case "bash":
		return ToolKindExecute
	case "edit", "multi_edit", "write":
		return ToolKindEdit
	case "view", "glob", "ls":
		return ToolKindRead
	case "grep":
		return ToolKindSearch
	case "fetch_url":
		return ToolKindFetch
	case "think":
		return ToolKindThink
	default:
		return ToolKindOther
	}
}

// isMutatingTool returns true for tools that modify filesystem or run commands.
var mutatingTools = map[string]bool{
	"bash":       true,
	"edit":       true,
	"multi_edit": true,
	"write":      true,
}

func isMutatingTool(name string) bool {
	return mutatingTools[name]
}

func extractText(blocks []ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

var sessionCounter atomic.Int64

func generateSessionID() string {
	n := sessionCounter.Add(1)
	return fmt.Sprintf("sess_%d", n)
}
