// Package gateway exposes standard OpenAI Chat Completions for manual and plugin sources.
package gateway

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/sourceplugin"
	"xiaotao/xt-client/internal/usage"
	"xiaotao/xt-client/internal/wireprotocol"
)

const maxBody = 32 << 20
const idleTimeout = 300 * time.Second
const totalTimeout = 30 * time.Minute

type Service struct {
	plugins *sourceplugin.Manager
	mu      sync.RWMutex
	store   *modelsource.Store
	key     string
	saveKey func(string) error
	usage   *usage.Service
	client  *http.Client
}

func New(store *modelsource.Store, key string, saveKey func(string) error) *Service {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = idleTimeout
	return &Service{store: store, key: key, saveKey: saveKey, client: &http.Client{Transport: transport, Timeout: totalTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("upstream redirect is disabled") }}}
}
func (s *Service) APIKey() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.key }
func (s *Service) RotateKey() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", errors.New("cannot generate gateway key")
	}
	key := "xt_" + hex.EncodeToString(b)
	if s.saveKey == nil || s.saveKey(key) != nil {
		return "", errors.New("cannot save gateway key")
	}
	s.key = key
	return key, nil
}
func (s *Service) Authorize(r *http.Request) bool {
	key := s.APIKey()
	header := r.Header.Get("Authorization")
	token := r.Header.Get("x-api-key")
	if strings.HasPrefix(header, "Bearer ") {
		token = strings.TrimPrefix(header, "Bearer ")
	} else if header != "" {
		return false
	}
	return key != "" && subtle.ConstantTimeCompare([]byte(token), []byte(key)) == 1
}
func (s *Service) SetUsage(u *usage.Service) { s.mu.Lock(); defer s.mu.Unlock(); s.usage = u }
func (s *Service) Models() []map[string]any {
	result := []map[string]any{}
	if s.store == nil {
		return result
	}
	sources := s.callSources()
	for _, src := range sources {
		if !src.Enabled || !src.HasAPIKey {
			continue
		}
		for _, m := range src.Models {
			entry := map[string]any{
				"id": m.CallID, "object": "model", "created": 0, "owned_by": src.ID,
				"label": m.Label, "source_id": src.ID, "source_label": src.Label,
				"upstream_model_id": m.UpstreamModelID,
				"supports_images":   m.SupportsImages, "supports_reasoning": m.SupportsReasoning,
				"reasoning_efforts": m.ReasoningEfforts,
			}
			if m.ContextWindow > 0 {
				entry["max_input_tokens"] = m.ContextWindow
			}
			if m.MaxOutputTokens > 0 {
				entry["max_output_tokens"] = m.MaxOutputTokens
			}
			if m.PriceFactor != nil {
				entry["price_factor"] = *m.PriceFactor
			}
			if m.IsFree != nil {
				entry["is_free"] = *m.IsFree
			}
			result = append(result, entry)
		}
	}
	return result
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "type": "gateway_error", "code": "gateway_error"}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func requestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}

func (s *Service) Test(ctx context.Context, sourceID string) error {
	if s.store == nil {
		return modelsource.ErrNotFound
	}
	src, err := s.store.Get(sourceID)
	if err != nil {
		return err
	}
	if !src.Enabled || len(src.Models) == 0 {
		return errors.New("source has no enabled model route")
	}
	resolved, m, key, err := s.store.Resolve(src.Models[0].ID)
	if err != nil {
		return err
	}
	body := map[string]any{"model": m.UpstreamModelID, "messages": []any{map[string]any{"role": "user", "content": "Reply OK"}}, "max_tokens": 16, "stream": false}
	resp, cancel, err := s.send(ctx, resolved, key, body)
	if err != nil {
		return errors.New("source connection failed")
	}
	defer cancel()
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("source rejected test request")
	}
	_, err = decodeCompletion(resp.Body)
	if err != nil {
		return errors.New("source returned an invalid chat completion")
	}
	return nil
}

func (s *Service) send(ctx context.Context, src modelsource.Source, key string, body map[string]any) (*http.Response, context.CancelFunc, error) {
	if src.Kind == "plugin" && s.plugins != nil {
		return s.pluginSend(ctx, src, body)
	}
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	native, err := wireprotocol.FromChatRequest(src.Protocol, body)
	if err != nil {
		cancel()
		return nil, func() {}, protocolRequestError{err}
	}
	raw, err := json.Marshal(native)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, modelsource.ProtocolURL(src.BaseURL, src.Protocol), strings.NewReader(string(raw)))
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	if src.Protocol == wireprotocol.Messages {
		req.Header.Set("x-api-key", key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	// One timer owns idle cancellation throughout headers and response reads. Closing the
	// wrapped body stops it; request cancellation unblocks the transport's reader.
	timer := time.AfterFunc(idleTimeout, cancel)
	resp, err := s.client.Do(req)
	if err != nil {
		timer.Stop()
		cancel()
		return nil, func() {}, errors.New("upstream request failed")
	}
	resp.Body = &idleBody{ReadCloser: resp.Body, timer: timer}
	stream, _ := body["stream"].(bool)
	if err := adaptUpstream(resp, src.Protocol, stream); err != nil {
		_ = resp.Body.Close()
		timer.Stop()
		cancel()
		return nil, func() {}, err
	}
	return resp, func() { timer.Stop(); cancel() }, nil
}

type idleBody struct {
	io.ReadCloser
	timer *time.Timer
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.timer.Reset(idleTimeout)
	}
	return n, err
}
func (b *idleBody) Close() error { b.timer.Stop(); return b.ReadCloser.Close() }
