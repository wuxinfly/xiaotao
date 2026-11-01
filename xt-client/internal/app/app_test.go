package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"xiaotao/xt-client/internal/runtimeenv"
	"xiaotao/xt-client/internal/secrets"
	"xiaotao/xt-client/internal/storage"
)

func TestConfigAndOrigin(t *testing.T) {
	dir := t.TempDir()
	s := State{URL: "http://127.0.0.1:12345", Token: "secret", PID: 42}
	h, e := Handler(s, dir, func() {})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if c, ok := h.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	call := func(method, path, origin, host, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, s.URL+path, strings.NewReader(body))
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	host := "127.0.0.1:12345"
	if w := call("PUT", "/api/config", "http://evil.example", host, "{}"); w.Code != 403 {
		t.Fatalf("cross origin: %d", w.Code)
	}
	if w := call("GET", "/api/status", "", "evil.example", ""); w.Code != 403 {
		t.Fatalf("host: %d", w.Code)
	}
	if w := call("PUT", "/api/config", s.URL, host, `{"name":"测试空间"}`); w.Code != 200 {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	// Second write exercises replacement of an existing file on Windows.
	if w := call("PUT", "/api/config", s.URL, host, `{"name":"再次保存"}`); w.Code != 200 {
		t.Fatalf("replace: %d", w.Code)
	}
	if c, ok := h.(interface{ Close() error }); ok {
		_ = c.Close()
	}
	h, e = Handler(s, dir, func() {})
	if e != nil {
		t.Fatal(e)
	}
	if w := call("GET", "/api/config", "", host, ""); !strings.Contains(w.Body.String(), "再次保存") {
		t.Fatal(w.Body.String())
	}
	if w := call("GET", "/", "", host, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "小涛") {
		t.Fatal("page missing")
	}
	// WebView video playback needs byte ranges for seeking and metadata reads.
	r := httptest.NewRequest("GET", s.URL+"/startup.mp4", nil)
	r.Header.Set("Range", "bytes=0-31")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.Len() != 32 || w.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("startup video range: %d %s (%d bytes)", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	if w := call("GET", "/v1/models", "", host, ""); w.Code != 401 {
		t.Fatalf("model endpoint without key: %d", w.Code)
	}
}
func TestCorruptConfigPreserved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte("broken"), 0600)
	if _, e := Handler(State{}, dir, func() {}); e == nil {
		t.Fatal("expected error")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "broken" {
		t.Fatal("overwrote invalid data")
	}
}

func TestDashboardEndpoints(t *testing.T) {
	dir := t.TempDir()
	s := State{URL: "http://127.0.0.1:12345", Token: "secret", PID: 42}
	h, e := Handler(s, dir, func() {})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if c, ok := h.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()

	host := "127.0.0.1:12345"

	// Cross-origin request blocked
	rCross := httptest.NewRequest("GET", s.URL+"/api/dashboard", nil)
	rCross.Host = host
	rCross.Header.Set("Origin", "http://malicious.com")
	wCross := httptest.NewRecorder()
	h.ServeHTTP(wCross, rCross)
	if wCross.Code != 403 {
		t.Fatalf("expected 403 for cross-origin /api/dashboard, got %d", wCross.Code)
	}

	// Same-origin request allowed
	rSame := httptest.NewRequest("GET", s.URL+"/api/dashboard", nil)
	rSame.Host = host
	rSame.Header.Set("Origin", s.URL)
	wSame := httptest.NewRecorder()
	h.ServeHTTP(wSame, rSame)
	if wSame.Code != 200 {
		t.Fatalf("expected 200 for /api/dashboard, got %d %s", wSame.Code, wSame.Body.String())
	}
	if !strings.Contains(wSame.Body.String(), "currentSession") || !strings.Contains(wSame.Body.String(), "heatmap") {
		t.Fatalf("expected dashboard snapshot in response, got %s", wSame.Body.String())
	}
}

func TestUATImportsProductionModelAccountsOnce(t *testing.T) {
	base := t.TempDir()
	oldActive, oldLocalAppData := runtimeenv.Active, os.Getenv("LOCALAPPDATA")
	runtimeenv.Active = runtimeenv.UAT
	t.Setenv("LOCALAPPDATA", base)
	t.Cleanup(func() { runtimeenv.Active = oldActive; os.Setenv("LOCALAPPDATA", oldLocalAppData) })
	production := filepath.Join(base, "XiaoTao")
	productionAccounts := filepath.Join(production, "accounts")
	productionKeyring := filepath.Join(production, "keys")
	source, err := storage.Open(productionAccounts)
	if err != nil {
		t.Fatal(err)
	}
	sourceKeyring, err := secrets.Open(productionKeyring)
	if err != nil {
		t.Fatal(err)
	}
	account := &storage.Account{ID: "trae:1", Provider: "trae", Nickname: "正式账号", Enabled: true, UsageTokens: 10, UsageCalls: 2}
	if err := source.UpsertAccount(account); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveSecret(account.ID, []byte(`{"accessToken":"token"}`), sourceKeyring.Encrypt); err != nil {
		t.Fatal(err)
	}
	if err := source.SaveSecret("proxy", []byte("production-proxy-key"), sourceKeyring.Encrypt); err != nil {
		t.Fatal(err)
	}
	sourceBefore := snapshotFiles(t, productionAccounts, productionKeyring)

	uat := filepath.Join(base, "XiaoTao-uat")
	dst, err := storage.Open(filepath.Join(uat, "accounts"))
	if err != nil {
		t.Fatal(err)
	}
	dstKeyring, err := secrets.Open(filepath.Join(uat, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	if err := importModelAccounts(dst, dstKeyring, uat); err != nil {
		t.Fatal(err)
	}
	imported, ok := dst.Account(account.ID)
	if !ok || imported.UsageTokens != 0 || imported.UsageCalls != 0 || imported.Quota != nil {
		t.Fatalf("unexpected imported account: %#v", imported)
	}
	raw, err := dst.LoadSecret(account.ID, dstKeyring.Decrypt)
	if err != nil || string(raw) != `{"accessToken":"token"}` {
		t.Fatal("account credential was not re-encrypted in UAT")
	}
	if _, err := dst.LoadSecret("proxy", dstKeyring.Decrypt); err == nil {
		t.Fatal("production proxy key was imported")
	}
	if got := snapshotFiles(t, productionAccounts, productionKeyring); got != sourceBefore {
		t.Fatal("production account files changed during import")
	}
	if err := importModelAccounts(dst, dstKeyring, uat); err != nil {
		t.Fatal(err)
	}
	accounts := dst.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("repeated import changed account count: %d", len(accounts))
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
			out.WriteString(path)
			out.WriteString(":")
			out.WriteString(info.ModTime().String())
			out.WriteString(":")
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
