package app

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGatewayCatalogForCopyMatchesPublicModels(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	body := `{"label":"我的接口","protocol":"openai-chat","baseURL":"https://example.test/v1","apiKey":"must-not-leak","models":[{"upstreamModelId":"中文/模型","contextWindow":262144,"maxOutputTokens":8192,"supportsImages":true,"supportsReasoning":true,"reasoningEfforts":["low","high"]}]}`
	if response := appHTTP(t, h, state, "POST", "/api/model-sources", body); response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	read := appHTTP(t, h, state, "GET", "/api/gateway/models", "")
	if read.Code != 200 || strings.Contains(read.Body.String(), "must-not-leak") {
		t.Fatal(read.Code, read.Body.String())
	}
	var copyList struct{ Models []map[string]any }
	if err := json.Unmarshal(read.Body.Bytes(), &copyList); err != nil {
		t.Fatal(err)
	}
	if len(copyList.Models) != 1 || copyList.Models[0]["id"] != "我的接口/中文%2F模型" || copyList.Models[0]["max_input_tokens"] != float64(262144) || copyList.Models[0]["max_output_tokens"] != float64(8192) {
		t.Fatal(copyList)
	}
	keyResponse := appHTTP(t, h, state, "GET", "/api/proxy/key", "")
	var key struct{ Key string }
	if err := json.Unmarshal(keyResponse.Body.Bytes(), &key); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", state.URL+"/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key.Key)
	public := httptest.NewRecorder()
	h.ServeHTTP(public, req)
	var publicList struct{ Data []map[string]any }
	if err := json.Unmarshal(public.Body.Bytes(), &publicList); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(copyList.Models, publicList.Data) {
		t.Fatalf("copy catalog differs from public catalog: %s", public.Body.String())
	}
	sourceID := copyList.Models[0]["source_id"]
	disabled := fmt.Sprintf(`{"label":"我的接口","protocol":"openai-chat","baseURL":"https://example.test/v1","enabled":false,"models":[{"upstreamModelId":"中文/模型"}]}`)
	if response := appHTTP(t, h, state, "PUT", "/api/model-sources/"+sourceID.(string), disabled); response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	read = appHTTP(t, h, state, "GET", "/api/gateway/models", "")
	if err := json.Unmarshal(read.Body.Bytes(), &copyList); err != nil {
		t.Fatal(err)
	}
	if len(copyList.Models) != 0 {
		t.Fatal("disabled model remains in copy catalog", copyList)
	}
}
