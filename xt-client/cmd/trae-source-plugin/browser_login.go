package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"xiaotao/xt-client/internal/providers/trae"
)

func browserLogin(ctx context.Context) (string, error) {
	return waitForBrowserLogin(ctx, openLoginBrowser)
}

func waitForBrowserLogin(ctx context.Context, open func(string) error) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	nonce := randomID()
	token := make(chan string, 1)
	var once sync.Once
	mux := http.NewServeMux()
	mux.HandleFunc("OPTIONS /authorize", func(w http.ResponseWriter, r *http.Request) {
		if !allowTraeCallback(w, r) || r.Header.Get("Origin") == "" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		if !allowTraeCallback(w, r) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		q := r.URL.Query()
		state := q.Get("login_trace_id")
		if state == "" {
			state = q.Get("loginTraceID")
		}
		if state == "" {
			state = q.Get("state")
		}
		if state == "" {
			state = q.Get("nonce")
		}
		refresh := trae.ExtractRefreshToken(r.URL.String())
		if state != nonce || refresh == "" {
			http.Error(w, "授权回调无效", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte("<!doctype html><meta charset=utf-8><title>Trae 授权</title><h2>授权已提交，请回到小涛查看结果。</h2>"))
		_ = http.NewResponseController(w).Flush()
		once.Do(func() { token <- refresh })
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	if err := open(trae.LoginURL("cn", listener.Addr().(*net.TCPAddr).Port, nonce)); err != nil {
		return "", errors.New("无法打开授权浏览器")
	}
	select {
	case refresh := <-token:
		return refresh, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func allowTraeCallback(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if origin != "https://"+trae.RegionOf("cn").AuthHost {
		return false
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Add("Vary", "Origin")
	return true
}
