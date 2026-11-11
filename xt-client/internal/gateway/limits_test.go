package gateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"xiaotao/xt-client/internal/usage"
)

func TestRequestLimitAndInvalidModelParametersNeverReachUpstream(t *testing.T) {
	called := make(chan struct{}, 1)
	svc, src, _ := setupGateway(t, func(w http.ResponseWriter, r *http.Request) { called <- struct{}{}; completion(w) })
	cases := []string{
		fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"%s"}]}`, src.Models[0].ID, strings.Repeat("x", maxBody)),
		fmt.Sprintf(`{"model":%q,"messages":[{}],"max_tokens":4096}`, src.Models[0].ID),
		fmt.Sprintf(`{"model":%q,"messages":[{}],"max_tokens":16.5}`, src.Models[0].ID),
		fmt.Sprintf(`{"model":%q,"messages":[{}],"reasoning_effort":"unknown"}`, src.Models[0].ID),
		fmt.Sprintf(`{"model":%q,"messages":[{}],"stream":"true"}`, src.Models[0].ID),
	}
	for i, body := range cases {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer local-key")
		w := httptest.NewRecorder()
		svc.HandleChat(w, r)
		if w.Code != 400 {
			t.Fatalf("case %d returned %d", i, w.Code)
		}
	}
	select {
	case <-called:
		t.Fatal("invalid request reached upstream")
	default:
	}
	r := chatRequest(src.Models[0].ID, false)
	r.Header.Set("Authorization", "local-key")
	w := httptest.NewRecorder()
	svc.HandleChat(w, r)
	if w.Code != 401 {
		t.Fatal("Bearer prefix was optional")
	}
}

func TestTotalTimeoutAbortsStreamAsInterrupted(t *testing.T) {
	stopped := make(chan struct{})
	svc, src, u := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	})
	svc.client.Timeout = 150 * time.Millisecond
	w := httptest.NewRecorder()
	svc.HandleChat(w, chatRequest(src.Models[0].ID, true))
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout did not abort upstream")
	}
	if strings.Contains(w.Body.String(), "[DONE]") {
		t.Fatal("timed out stream claimed success")
	}
	if rec := summary(t, u); rec.Status != usage.StatusInterrupted {
		t.Fatal("timeout misclassified", rec.Status)
	}
}

func TestExplicitCompletionLimitIsNotOverridden(t *testing.T) {
	received := make(chan string, 1)
	svc, src, _ := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- string(body)
		completion(w)
	})
	raw := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}],"max_completion_tokens":64}`, src.Models[0].ID)
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw))
	r.Header.Set("Authorization", "Bearer local-key")
	w := httptest.NewRecorder()
	svc.HandleChat(w, r)
	body := <-received
	if w.Code != 200 || strings.Contains(body, `"max_tokens"`) || !strings.Contains(body, `"max_completion_tokens":64`) {
		t.Fatal("explicit output limit was changed", w.Code, body)
	}
}
