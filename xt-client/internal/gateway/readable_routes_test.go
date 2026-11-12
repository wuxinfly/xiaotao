package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadableManualGatewayRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			svc, src, _ := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["model"] != "provider/model" {
					t.Errorf("wrong upstream: %v", body["model"])
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				} else {
					completion(w)
				}
			})
			id, _ := svc.Models()[0]["id"].(string)
			if id != "test/provider%2Fmodel" || id != src.Models[0].CallID {
				t.Fatal(id)
			}
			w := httptest.NewRecorder()
			svc.HandleChat(w, chatRequest(id, stream))
			if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
