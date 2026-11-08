package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"xiaotao/xt-client/internal/logging"
	"xiaotao/xt-client/internal/usage"
)

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func genReqID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("req_%08x", time.Now().UnixNano()&0xffffffff)
	}
	return "req_" + hex.EncodeToString(b)
}

type staticStats struct {
	mu    sync.Mutex
	count int
}

var globalStaticStats staticStats

func isStaticAsset(path string) bool {
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/icons/") || path == "/favicon.ico" {
		return true
	}
	for _, ext := range []string{".js", ".css", ".png", ".svg", ".ico", ".woff2", ".ttf"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func accessLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := genReqID()
		ctx := logging.WithReqID(r.Context(), reqID)
		r = r.WithContext(ctx)

		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		start := time.Now()

		next.ServeHTTP(lw, r)

		duration := time.Since(start)
		logger := logging.Default().For(logging.ModuleClient)

		if isStaticAsset(r.URL.Path) {
			globalStaticStats.mu.Lock()
			globalStaticStats.count++
			c := globalStaticStats.count
			globalStaticStats.mu.Unlock()
			if c%50 == 0 {
				logger.Debug("static assets served", logging.F("total_count", c))
			}
			return
		}

		logger.Info("http request",
			logging.F("method", r.Method),
			logging.F("path", r.URL.Path),
			logging.F("status", lw.status),
			logging.F("duration_ms", duration.Milliseconds()),
			logging.F("bytes", lw.bytes),
			logging.F("req", reqID),
		)
	})
}

type closeableHandler struct {
	http.Handler
	usage *usage.Service
}

func (c *closeableHandler) Close() error {
	if c.usage != nil {
		return c.usage.Close()
	}
	return nil
}

func writeProxyError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": message, "type": code}})
}
