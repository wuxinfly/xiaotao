package sourceplugin

import (
	"encoding/json"
	"errors"
	"strings"
)

// Error carries a stable code across the plugin, HTTP and UI boundaries.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

const (
	CodePlugin      = "plugin_error"
	CodeUnavailable = "unavailable"
	CodeAuthExpired = "auth_expired"
	CodeRateLimited = "rate_limited"
	CodeUnsupported = "unsupported"
	CodeInvalid     = "invalid"
	CodeTimeout     = "timeout"
	CodeUpstream    = "upstream"
)

// Reserved JSON-RPC error codes for plugin-reported failures.
const (
	RPCPluginError  = -32000
	RPCAuthExpired  = -32001
	RPCRateLimited  = -32002
	RPCUnsupported  = -32003
	RPCInvalidInput = -32004
	RPCUpstream     = -32005
	RPCTimeout      = -32006
)

func NewError(code, message string) *Error { return &Error{Code: code, Message: message} }

func ErrorCode(err error) string {
	var target *Error
	if errors.As(err, &target) && target.Code != "" {
		return target.Code
	}
	return CodePlugin
}

func rpcFailure(e *RPCError) error {
	code := CodePlugin
	switch e.Code {
	case RPCAuthExpired:
		code = CodeAuthExpired
	case RPCRateLimited:
		code = CodeRateLimited
	case RPCUnsupported:
		code = CodeUnsupported
	case RPCInvalidInput:
		code = CodeInvalid
	case RPCUpstream:
		code = CodeUpstream
	case RPCTimeout:
		code = CodeTimeout
	}
	if len(e.Data) > 0 {
		var data struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(e.Data, &data) == nil {
			switch data.Code {
			case CodeAuthExpired, CodeRateLimited, CodeUnsupported, CodeInvalid, CodeTimeout, CodeUpstream, CodePlugin:
				code = data.Code
			}
		}
	}
	message := strings.TrimSpace(e.Message)
	if len(message) > 300 {
		message = message[:300]
	}
	return &Error{Code: code, Message: message}
}
