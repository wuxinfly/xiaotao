// Package plugin is the SDK for XiaoTao model-source plugins. It implements the
// JSON-RPC 2.0 stdio loop so a plugin only supplies a handler.
//
// Wire compatibility is covered by internal/sourceplugin/sdk_compat_test.go; keep
// both definitions in sync when the protocol changes.
package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
)

// MaxMessage mirrors the host's message limit.
const MaxMessage = 32 << 20

// ContractVersion is the protocol version this SDK implements.
const ContractVersion = 2

// Message is one newline-delimited JSON-RPC 2.0 message.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is a protocol error. Use the reserved codes so the host can show an
// actionable message instead of a generic failure.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Reserved error codes understood by the host.
const (
	CodePluginError  = -32000
	CodeAuthExpired  = -32001
	CodeRateLimited  = -32002
	CodeUnsupported  = -32003
	CodeInvalidInput = -32004
	CodeUpstream     = -32005
	CodeTimeout      = -32006
)

func (e *RPCError) Error() string { return e.Message }

func Errorf(code int, message string) error { return &RPCError{Code: code, Message: message} }

// Operation is the params object sent with every method call.
type Operation struct {
	Credential json.RawMessage   `json:"credential,omitempty"`
	Input      json.RawMessage   `json:"input,omitempty"`
	Body       map[string]any    `json:"body,omitempty"`
	Model      string            `json:"model,omitempty"`
	Settings   map[string]string `json:"settings,omitempty"`
}

// Model describes one model offered by the plugin.
type Model struct {
	Key               string   `json:"key"`
	UpstreamID        string   `json:"upstreamId"`
	Label             string   `json:"label"`
	ContextWindow     int      `json:"contextWindow,omitempty"`
	SupportsImages    bool     `json:"supportsImages"`
	SupportsReasoning bool     `json:"supportsReasoning"`
	ReasoningEfforts  []string `json:"reasoningEfforts,omitempty"`
	PriceFactor       *float64 `json:"priceFactor,omitempty"`
	IsFree            *bool    `json:"isFree,omitempty"`
}

// AccountInfo is the stable platform identity of a credential.
type AccountInfo struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Result is what a handler returns. Only fill the fields the method produces.
type Result struct {
	Account    *AccountInfo    `json:"account,omitempty"`
	Credential json.RawMessage `json:"credential,omitempty"`
	Models     []Model         `json:"models,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
	Display    *Display        `json:"display,omitempty"`
}

// Display is presentation data the host renders as plain text.
type Display struct {
	Summary string        `json:"summary,omitempty"`
	Tone    string        `json:"tone,omitempty"`
	Items   []DisplayItem `json:"items,omitempty"`
}

type DisplayItem struct {
	Label   string   `json:"label"`
	Value   string   `json:"value"`
	Percent *float64 `json:"percent,omitempty"`
	Tone    string   `json:"tone,omitempty"`
}

// LoginDescriptor describes a host-driven authorization flow. The host replaces
// {state} and {callback} in URL, opens the browser and validates the callback.
type LoginDescriptor struct {
	URL          string `json:"url"`
	StateParam   string `json:"stateParam,omitempty"`
	ResultParam  string `json:"resultParam"`
	Callback     string `json:"callback,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// Handler serves one method. Returning an error ends the request; return a
// *RPCError to control the code the host reports.
type Handler func(ctx context.Context, op Operation) (Result, error)

// Server wires a plugin identity to method handlers.
type Server struct {
	ID              string
	ContractVersion int
	Handlers        map[string]Handler
	// Emit sends a streaming notification. Set by Serve.
	Emit func(method string, params any) error
}

// Serve runs the stdio loop until stdin closes. It returns nil on clean EOF.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	if s.ContractVersion == 0 {
		s.ContractVersion = ContractVersion
	}
	encoder := json.NewEncoder(out)
	s.Emit = func(method string, params any) error {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		return encoder.Encode(Message{JSONRPC: "2.0", Method: method, Params: raw})
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	for scanner.Scan() {
		var msg Message
		if json.Unmarshal(scanner.Bytes(), &msg) != nil || msg.JSONRPC != "2.0" || msg.ID == 0 {
			return errors.New("invalid request")
		}
		response := Message{JSONRPC: "2.0", ID: msg.ID}
		if msg.Method == "initialize" {
			raw, _ := json.Marshal(map[string]any{"id": s.ID, "contractVersion": s.ContractVersion})
			response.Result = raw
		} else if handler := s.Handlers[msg.Method]; handler != nil {
			var op Operation
			if json.Unmarshal(msg.Params, &op) != nil {
				response.Error = &RPCError{Code: CodeInvalidInput, Message: "invalid operation"}
			} else if result, err := handler(context.Background(), op); err != nil {
				response.Error = asRPCError(err)
			} else if raw, err := json.Marshal(result); err != nil {
				response.Error = &RPCError{Code: CodePluginError, Message: "cannot encode result"}
			} else {
				response.Result = raw
			}
		} else {
			response.Error = &RPCError{Code: CodeUnsupported, Message: "unsupported method"}
		}
		if encoder.Encode(response) != nil {
			return errors.New("write failed")
		}
	}
	return scanner.Err()
}

// Main is a convenience entry point for simple plugins.
func (s *Server) Main() {
	if err := s.Serve(os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}

func asRPCError(err error) *RPCError {
	var rpc *RPCError
	if errors.As(err, &rpc) {
		return rpc
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &RPCError{Code: CodeTimeout, Message: "timeout"}
	}
	return &RPCError{Code: CodePluginError, Message: err.Error()}
}
