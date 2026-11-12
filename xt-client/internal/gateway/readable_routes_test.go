package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/modelsource"
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

func TestChineseGatewayIDsRemainReadableAndAcceptEncodedLegacy(t *testing.T) {
	for _, stream := range []bool{false, true} {
		svc, _, _ := setupGateway(t, func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["model"] != "中文/模型" {
				t.Errorf("wrong upstream: %v", body["model"])
			}
			if stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
			} else {
				completion(w)
			}
		})
		key := "secret"
		src, err := svc.store.Save("", modelsource.Input{Label: "我的接口", Protocol: "openai-chat", BaseURL: svc.store.List()[0].BaseURL, APIKey: &key, Models: []modelsource.Model{{UpstreamModelID: "中文/模型"}}})
		if err != nil {
			t.Fatal(err)
		}
		if svc.Models()[1]["id"] != "我的接口/中文%2F模型" {
			t.Fatal(svc.Models())
		}
		for _, id := range []string{src.Models[0].CallID, "%E6%88%91%E7%9A%84%E6%8E%A5%E5%8F%A3/%E4%B8%AD%E6%96%87%2F%E6%A8%A1%E5%9E%8B", src.Models[0].ID} {
			w := httptest.NewRecorder()
			svc.HandleChat(w, chatRequest(id, stream))
			if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
				t.Fatal(w.Code, w.Body.String())
			}
		}
	}
}
