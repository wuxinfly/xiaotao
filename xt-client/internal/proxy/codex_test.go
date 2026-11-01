package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

func TestCodexProxyRoutingAndDispatch(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	
	acct := &storage.Account{
		ID:        "codex:global:test-user",
		Provider:  "codex",
		Region:    "global",
		UID:       "test-user",
		Nickname:  "Codex User",
		Enabled:   true,
		AuthState: "valid",
	}
	if err := store.UpsertAccount(acct); err != nil {
		t.Fatalf("UpsertAccount failed: %v", err)
	}
	if err := store.SaveSecret(acct.ID, []byte("dummy"), func(b []byte) ([]byte, error) { return b, nil }); err != nil {
		t.Fatalf("SaveSecret failed: %v", err)
	}

	cred := providers.Credential{
		AccessToken:  "codex-at",
		RefreshToken: "codex-rt",
		UID:          "test-user",
	}
	rawCred, _ := json.Marshal(cred)

	secret := func(id string) ([]byte, error) {
		return rawCred, nil
	}

	svc := New(store, secret, "test-api-key")

	// Verify Model listing
	models, err := svc.Models()
	if err != nil {
		t.Fatalf("svc.Models failed: %v", err)
	}
	foundSol := false
	for _, m := range models {
		if id, ok := m["id"].(string); ok && id == "codex/gpt-6.1-sol" {
			foundSol = true
			break
		}
	}
	if !foundSol {
		t.Fatalf("expected codex/gpt-6.1-sol in model catalog, got: %+v", models)
	}

	// Verify SelectAccount
	selectedAcct, modelID, ok := svc.SelectAccount("codex/gpt-6.1-sol")
	if !ok || selectedAcct.ID != acct.ID || modelID != "gpt-6.1-sol" {
		t.Fatalf("SelectAccount failed: ok=%v, acct=%+v, modelID=%s", ok, selectedAcct, modelID)
	}

	// Test empty or non-ready accounts
	noAcct, _, okNo := svc.SelectAccount("codex/non-existent-model")
	if okNo {
		t.Fatalf("expected false for unknown model, got %+v", noAcct)
	}

	// When proxying request to chat with valid model
	reqBody := `{"model":"codex/gpt-6.1-sol","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer test-api-key")
	rec := httptest.NewRecorder()

	// HandleChat will try to talk to codex collector; without a real network it should return 502 upstream_error
	svc.HandleChat(rec, req)
	if rec.Code != http.StatusBadGateway && rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 502 or 401 for mock network fail, got %d: %s", rec.Code, rec.Body.String())
	}
}
