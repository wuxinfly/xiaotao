package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
	"xiaotao/xt-client/internal/desktop"
	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/runtimeenv"
)

// Serve runs the HTTP server and blocks until ctx is canceled or an error occurs.
func Serve(ctx context.Context, data string) error {
	release, e := desktop.Lock(runtimeenv.ServiceMutex(Key(data)))
	if e != nil {
		return e
	}
	defer release()
	defaultPort := runtimeenv.Current().DefaultPort
	l, e := net.Listen("tcp4", "127.0.0.1:"+defaultPort)
	if e != nil {
		l, e = net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			logging.Default().For(logging.ModuleClient).Error("server listen failed", logging.F("error", e))
			return e
		}
		actualPort := l.Addr().(*net.TCPAddr).Port
		logging.Default().For(logging.ModuleClient).Warn("default port occupied, fallback to random port",
			logging.F("default_port", defaultPort),
			logging.F("actual_port", actualPort),
		)
	} else {
		actualPort := l.Addr().(*net.TCPAddr).Port
		logging.Default().For(logging.ModuleClient).Info("server listening", logging.F("port", actualPort))
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
	if closer, ok := h.(interface{ Close() error }); ok {
		defer closer.Close()
	}
	srv := http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	if e = Save(filepath.Join(data, "runtime.json"), s); e != nil {
		logging.Default().For(logging.ModuleClient).Error("save runtime.json failed", logging.F("error", e))
		return e
	}
	logging.Default().For(logging.ModuleClient).Info("runtime state written", logging.F("url", s.URL), logging.F("pid", s.PID))
	defer os.Remove(filepath.Join(data, "runtime.json"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = srv.Shutdown(c)
	}()
	e = srv.Serve(l)
	cancel()
	<-done
	if e == http.ErrServerClosed {
		logging.Default().For(logging.ModuleClient).Info("server stopped", logging.F("reason", "shutdown"))
		return nil
	}
	if e != nil {
		logging.Default().For(logging.ModuleClient).Error("server stopped with error", logging.F("error", e))
	} else {
		logging.Default().For(logging.ModuleClient).Info("server stopped", logging.F("reason", "clean_exit"))
	}
	return e
}
