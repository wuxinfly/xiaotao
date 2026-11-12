package sourceplugin

import (
	"context"
	"encoding/json"
	"errors"
)

// LoginDescriptor is what a plugin returns for login/prepare. The host owns the
// browser, the loopback callback listener and state validation; the plugin only
// describes the URL and which query parameter carries the result.
type LoginDescriptor struct {
	URL          string `json:"url"`
	StateParam   string `json:"stateParam,omitempty"`
	ResultParam  string `json:"resultParam"`
	Callback     string `json:"callback,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// PrepareLogin asks the plugin for a host-driven authorization descriptor.
func (m *Manager) PrepareLogin(ctx context.Context, id, accountID string) (LoginDescriptor, error) {
	if !m.Supports(id, "host-login") {
		return LoginDescriptor{}, NewError(CodeUnsupported, "插件不支持由宿主完成的登录")
	}
	result, err := m.RunForAccount(ctx, id, accountID, "login/prepare", nil, nil, "", nil)
	if err != nil {
		return LoginDescriptor{}, err
	}
	var descriptor LoginDescriptor
	if len(result.Value) == 0 || json.Unmarshal(result.Value, &descriptor) != nil {
		return LoginDescriptor{}, NewError(CodePlugin, "插件没有返回有效的登录信息")
	}
	if descriptor.URL == "" || descriptor.ResultParam == "" {
		return LoginDescriptor{}, NewError(CodePlugin, "插件没有返回有效的登录信息")
	}
	return descriptor, nil
}

// CompleteLogin hands the authorization result back to the plugin.
func (m *Manager) CompleteLogin(ctx context.Context, id, accountID string, input json.RawMessage) (Result, error) {
	if !m.Supports(id, "host-login") {
		return Result{}, NewError(CodeUnsupported, "插件不支持由宿主完成的登录")
	}
	return m.RunForAccount(ctx, id, accountID, "login/complete", input, nil, "", nil)
}

// ErrLoginCancelled is returned when the host stops waiting for a callback.
var ErrLoginCancelled = errors.New("login cancelled")
