package sourceplugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HostLogin runs a host-owned authorization flow. The plugin describes the
// authorization URL and which query parameters carry state and the result; the
// host owns the loopback listener, state validation and browser launch.
//
// The descriptor URL may contain {callback} and {state}, which the host fills in.
func HostLogin(ctx context.Context, plugins *Manager, id string, open func(string) error) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	descriptor, err := plugins.PrepareLogin(ctx, id, "")
	if err != nil {
		return Result{}, err
	}
	path := descriptor.Callback
	if path == "" {
		path = "/callback"
	}
	if !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#") {
		return Result{}, NewError(CodeInvalid, "插件登录回调路径无效")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Result{}, NewError(CodeUnavailable, "无法启动登录回调监听")
	}
	defer listener.Close()
	state := loginNonce()
	callback := "http://" + listener.Addr().String() + path
	authURL := strings.NewReplacer("{callback}", url.QueryEscape(callback), "{state}", url.QueryEscape(state)).Replace(descriptor.URL)
	if parsed, err := url.Parse(authURL); err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return Result{}, NewError(CodeInvalid, "插件登录地址无效")
	}

	received := make(chan map[string]string, 1)
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if descriptor.StateParam != "" && query.Get(descriptor.StateParam) != state {
			http.Error(w, "登录状态无效", http.StatusBadRequest)
			return
		}
		values := map[string]string{}
		for key, list := range query {
			if len(list) > 0 && len(list[0]) <= 4096 {
				values[key] = list[0]
			}
		}
		if descriptor.ResultParam != "" && values[descriptor.ResultParam] == "" {
			http.Error(w, "登录结果缺失", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><title>授权完成</title><h2>授权已提交，请回到小涛查看结果。</h2>"))
		once.Do(func() { received <- values })
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()

	if err := open(authURL); err != nil {
		return Result{}, NewError(CodeUnavailable, "无法打开授权浏览器")
	}
	var values map[string]string
	select {
	case values = <-received:
	case <-ctx.Done():
		return Result{}, ErrLoginCancelled
	}
	input, err := json.Marshal(map[string]any{"callback": values})
	if err != nil {
		return Result{}, NewError(CodeInvalid, "登录结果无效")
	}
	return plugins.CompleteLogin(ctx, id, "", input)
}

func loginNonce() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))[:32]
	}
	return hex.EncodeToString(raw)
}
