package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/runtimeenv"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
)

type sourceTestHandler struct {
	http.Handler
	once sync.Once
}

func (h *sourceTestHandler) Close() error {
	var err error
	h.once.Do(func() { err = h.Handler.(interface{ Close() error }).Close() })
	return err
}

func newSourceTestApp(t *testing.T, dir string) (http.Handler, State) {
	t.Helper()
	state := State{URL: "http://127.0.0.1:12345", Token: "management-token", PID: 42}
	h, err := Handler(state, dir, func() {})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &sourceTestHandler{Handler: h}
	t.Cleanup(func() { _ = wrapped.Close() })
	return wrapped, state
}

func appHTTP(t *testing.T, h http.Handler, state State, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, state.URL+path, strings.NewReader(body))
	r.Header.Set("Origin", state.URL)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestConfigAndOrigin(t *testing.T) {
	dir := t.TempDir()
	h, state := newSourceTestApp(t, dir)
	r := httptest.NewRequest("PUT", state.URL+"/api/config", strings.NewReader("{}"))
	r.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	r = httptest.NewRequest("GET", state.URL+"/api/status", nil)
	r.Host = "evil.example"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("host: %d", w.Code)
	}
	for _, name := range []string{"测试空间", "再次保存"} {
		w = appHTTP(t, h, state, "PUT", "/api/config", `{"name":"`+name+`"}`)
		if w.Code != 200 {
			t.Fatalf("save: %d %s", w.Code, w.Body.String())
		}
	}
	if closer, ok := h.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	h, state = newSourceTestApp(t, dir)
	if w = appHTTP(t, h, state, "GET", "/api/config", ""); !strings.Contains(w.Body.String(), "再次保存") {
		t.Fatal(w.Body.String())
	}
	if w = appHTTP(t, h, state, "GET", "/", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "小涛") {
		t.Fatalf("page missing: %d", w.Code)
	}
	r = httptest.NewRequest("GET", state.URL+"/startup.mp4", nil)
	r.Header.Set("Range", "bytes=0-31")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.Len() != 32 || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("video range: %d %d", w.Code, w.Body.Len())
	}
	if w = appHTTP(t, h, state, "GET", "/v1/models", ""); w.Code != 401 {
		t.Fatalf("models without key: %d", w.Code)
	}
}

func TestCorruptConfigPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Handler(State{}, dir, func() {}); err == nil {
		t.Fatal("expected error")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "broken" {
		t.Fatal("overwrote invalid data")
	}
}

func TestDashboardEndpoints(t *testing.T) {
	h, state := newSourceTestApp(t, t.TempDir())
	r := httptest.NewRequest("GET", state.URL+"/api/dashboard", nil)
	r.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	w = appHTTP(t, h, state, "GET", "/api/dashboard", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "currentSession") || !strings.Contains(w.Body.String(), "heatmap") {
		t.Fatal(w.Body.String())
	}
}

func TestPlatformRoutesRemovedAndLegacyDataPreserved(t *testing.T) {
	dir := t.TempDir()
	old, err := storage.Open(filepath.Join(dir, "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	if err = old.UpsertAccount(&storage.Account{ID: "trae:1", Provider: "trae", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	keyring, err := secrets.Open(filepath.Join(dir, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	if err = old.SaveSecret("trae:1", []byte("legacy-token"), keyring.Encrypt); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, filepath.Join(dir, "accounts"))
	h, state := newSourceTestApp(t, dir)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/auth/trae"}, {"POST", "/api/auth/codex/import"}, {"GET", "/api/auth/workbuddy/session"},
		{"GET", "/api/providers"}, {"POST", "/api/accounts/order"}, {"POST", "/api/accounts/trae:1/rename"},
		{"POST", "/api/accounts/trae:1/quota"}, {"GET", "/api/accounts/trae:1/checkin"}, {"POST", "/api/checkin/claim-all"}, {"POST", "/api/proxy/models"},
	} {
		if w := appHTTP(t, h, state, tc.method, tc.path, "{}"); w.Code != 404 {
			t.Fatalf("removed %s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if w := appHTTP(t, h, state, "GET", "/api/accounts", ""); w.Code != 200 || strings.Contains(w.Body.String(), "trae:1") {
		t.Fatal(w.Body.String())
	}
	if after := snapshotFiles(t, filepath.Join(dir, "accounts")); before != after {
		t.Fatal("legacy platform accounts modified")
	}
}

func TestUATImportsManualSourcesOnceAndReencrypts(t *testing.T) {
	base := t.TempDir()
	oldEnv := runtimeenv.Active
	runtimeenv.Active = runtimeenv.UAT
	t.Cleanup(func() { runtimeenv.Active = oldEnv })
	t.Setenv("LOCALAPPDATA", base)
	production := filepath.Join(base, "XiaoTao")
	sourceKeys, err := secrets.Open(filepath.Join(production, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	source, err := modelsource.Open(production, sourceKeys.Encrypt, sourceKeys.Decrypt)
	if err != nil {
		t.Fatal(err)
	}
	key := "production-upstream-key"
	saved, err := source.Save("", modelsource.Input{Label: "正式接口", BaseURL: "https://api.example/v1", Protocol: "openai-chat", APIKey: &key, Models: []modelsource.Model{{UpstreamModelID: "org/model", ContextWindow: 32000, MaxOutputTokens: 2048}}})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, filepath.Join(production, "model-sources"), filepath.Join(production, "keys"))
	uat := filepath.Join(base, "XiaoTao-uat")
	newSourceTestApp(t, uat)
	targetKeys, err := secrets.Open(filepath.Join(uat, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := modelsource.Open(uat, targetKeys.Encrypt, targetKeys.Decrypt)
	if err != nil {
		t.Fatal(err)
	}
	got, model, plain, err := target.Resolve(saved.Models[0].ID)
	if err != nil || plain != key || got.ID != saved.ID || model.MaxOutputTokens != 2048 {
		t.Fatalf("import: %+v %+v %q %v", got, model, plain, err)
	}
	if before != snapshotFiles(t, filepath.Join(production, "model-sources"), filepath.Join(production, "keys")) {
		t.Fatal("production files changed")
	}
	productionRaw, _ := os.ReadFile(filepath.Join(production, "model-sources", "sources.json"))
	uatRaw, _ := os.ReadFile(filepath.Join(uat, "model-sources", "sources.json"))
	if bytes.Equal(productionRaw, uatRaw) {
		t.Fatal("credentials were copied without re-encryption")
	}
	if err = target.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	if err = importManualModelSources(target, targetKeys, uat); err != nil {
		t.Fatal(err)
	}
	if len(target.List()) != 0 {
		t.Fatal("deleted source reimported")
	}
	// An explicitly empty production configuration must still complete import,
	// even when the new UAT has no model-sources directory yet.
	if err = source.Delete(saved.ID); err != nil {
		t.Fatal(err)
	}
	emptyUAT := filepath.Join(base, "XiaoTao-empty-uat")
	newSourceTestApp(t, emptyUAT)
	if _, err = os.Stat(filepath.Join(emptyUAT, "model-sources", "import.json")); err != nil {
		t.Fatalf("empty import marker missing: %v", err)
	}
}

func TestGatewayKeyPreservedAndIndependent(t *testing.T) {
	dir := t.TempDir()
	keys, err := secrets.Open(filepath.Join(dir, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := storage.Open(filepath.Join(dir, "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	oldKey := strings.Repeat("a", 48)
	if err = legacy.SaveSecret("proxy", []byte(oldKey), keys.Encrypt); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, filepath.Join(dir, "accounts"))
	h, state := newSourceTestApp(t, dir)
	w := appHTTP(t, h, state, "GET", "/api/proxy/key", "")
	var initial struct{ Key string }
	_ = json.Unmarshal(w.Body.Bytes(), &initial)
	if initial.Key != oldKey {
		t.Fatalf("old gateway key lost: %s", w.Body.String())
	}
	w = appHTTP(t, h, state, "POST", "/api/proxy/key", "{}")
	var rotated struct{ Key string }
	_ = json.Unmarshal(w.Body.Bytes(), &rotated)
	if w.Code != 200 || rotated.Key == oldKey || len(strings.TrimPrefix(rotated.Key, "xt_")) != 48 {
		t.Fatal(w.Body.String())
	}
	if before != snapshotFiles(t, filepath.Join(dir, "accounts")) {
		t.Fatal("key rotation modified frozen account file")
	}
	h, state = newSourceTestApp(t, dir)
	w = appHTTP(t, h, state, "GET", "/api/proxy/key", "")
	if !strings.Contains(w.Body.String(), rotated.Key) {
		t.Fatal("gateway key rotation not persisted")
	}
}

func snapshotFiles(t *testing.T, dirs ...string) string {
	t.Helper()
	var out bytes.Buffer
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			out.WriteString(path + ":" + info.ModTime().String() + ":")
			if !entry.IsDir() {
				raw, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				sum := sha256.Sum256(append([]byte(path), raw...))
				out.WriteString(hex.EncodeToString(sum[:]))
			}
			out.WriteByte('\n')
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out.String()
}
