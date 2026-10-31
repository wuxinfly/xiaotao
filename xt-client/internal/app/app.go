package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"xiaotao/xt-client/internal/desktop"
)

const Version = "0.1.0-dev"

//go:embed web/*
var assets embed.FS

type State struct {
	URL     string
	Token   string
	PID     int
	Version string
}
type Config struct {
	Name string `json:"name"`
}

func Key(data string) string { h := sha256.Sum256([]byte(data)); return hex.EncodeToString(h[:12]) }
func Save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func Read(data string) (State, error) {
	var s State
	b, e := os.ReadFile(filepath.Join(data, "runtime.json"))
	if e == nil {
		e = json.Unmarshal(b, &s)
	}
	return s, e
}
func Alive(s State) bool {
	c := http.Client{Timeout: time.Second}
	req, e := http.NewRequest("GET", s.URL+"/health", nil)
	if e != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	r, e := c.Do(req)
	if e != nil {
		return false
	}
	defer r.Body.Close()
	var v struct {
		App string
		PID int
	}
	return r.StatusCode == 200 && json.NewDecoder(r.Body).Decode(&v) == nil && v.App == "xiaotao" && v.PID == s.PID
}
func Handler(s State, data string, stop func()) (http.Handler, error) {
	cfg := Config{Name: "小涛"}
	path := filepath.Join(data, "config.json")
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &cfg); e != nil {
			return nil, fmt.Errorf("invalid config: %w", e)
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else if e = Save(path, cfg); e != nil {
		return nil, e
	}
	var mu sync.Mutex
	mux := http.NewServeMux()
	started := time.Now()
	send := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"App": "xiaotao", "PID": s.PID, "version": Version})
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		send(w, map[string]any{"version": Version, "url": s.URL, "pid": s.PID, "uptime": int(time.Since(started).Seconds()), "dataDir": data, "modelReady": false})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { mu.Lock(); defer mu.Unlock(); send(w, cfg) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var next Config
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&next); e != nil || len([]rune(next.Name)) < 1 || len([]rune(next.Name)) > 40 {
			http.Error(w, "invalid name", 400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if e := Save(path, next); e != nil {
			http.Error(w, "save failed", 500)
			return
		}
		cfg = next
		send(w, cfg)
	})
	mux.HandleFunc("POST /api/shutdown", func(w http.ResponseWriter, r *http.Request) { send(w, map[string]bool{"ok": true}); go stop() })
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		send(w, map[string]any{"error": map[string]string{"message": "模型代理尚未实现", "type": "not_implemented"}})
	})
	sub, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.URL[len("http://"):] {
			http.Error(w, "invalid host", 403)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Authorization") != "Bearer "+s.Token && r.Header.Get("Origin") != s.URL {
			http.Error(w, "same-origin required", 403)
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}
func Serve(ctx context.Context, data string) error {
	release, e := desktop.Lock("Local\\XiaoTao.Service." + Key(data))
	if e != nil {
		return e
	}
	defer release()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	defer l.Close()
	token := make([]byte, 32)
	if _, e = rand.Read(token); e != nil {
		return e
	}
	s := State{URL: "http://" + l.Addr().String(), Token: hex.EncodeToString(token), PID: os.Getpid(), Version: Version}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	h, e := Handler(s, data, cancel)
	if e != nil {
		return e
	}
	srv := http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if e = Save(filepath.Join(data, "runtime.json"), s); e != nil {
		return e
	}
	defer os.Remove(filepath.Join(data, "runtime.json"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		srv.Shutdown(c)
	}()
	e = srv.Serve(l)
	cancel()
	<-done
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
