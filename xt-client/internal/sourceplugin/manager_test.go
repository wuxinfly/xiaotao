package sourceplugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("XT_PLUGIN_TEST_CHILD") == "1" {
		helper()
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func helper() {
	scanner := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req Message
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			return
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{"id": "test.fake", "contractVersion": 1}
		case "login/import":
			result = Result{Credential: json.RawMessage(`{"token":"private"}`)}
		case "login/prepare":
			result = Result{Value: json.RawMessage(`{"url":"http://example.test/auth?state={state}&cb={callback}","stateParam":"state","resultParam":"token","callback":"/cb"}`)}
		case "login/complete":
			var op Operation
			_ = json.Unmarshal(req.Params, &op)
			var input struct {
				Callback map[string]string `json:"callback"`
			}
			_ = json.Unmarshal(op.Input, &input)
			result = Result{Account: &AccountInfo{ID: "user-" + input.Callback["token"], Name: "Host Login"}, Credential: json.RawMessage(`{"token":"` + input.Callback["token"] + `"}`)}
		case "quota/get":
			result = Result{Value: json.RawMessage(`{"summary":"from plugin"}`), Display: &Display{Summary: "总额度 10 / 100", Items: []DisplayItem{{Label: "总额度", Value: "10 / 100", Percent: floatPtr(10)}}}}
		case "models/refresh":
			factor := 0.31
			result = Result{Models: []Model{{Key: "stable", UpstreamID: "org/model", Label: "Example", PriceFactor: &factor}}}
		case "diag":
			fmt.Fprintln(os.Stderr, "diagnostic line")
			result = Result{}
		case "wait":
			time.Sleep(time.Minute)
		case "crash":
			os.Exit(2)
		default:
			result = Result{}
		}
		raw, _ := json.Marshal(result)
		_ = out.Encode(Message{JSONRPC: "2.0", ID: req.ID, Result: raw})
	}
}
func fixture(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("XT_PLUGIN_TEST_CHILD", "1")
	pkg := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(filepath.Join(pkg, "provider.exe"), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
	raw := []byte(`{"id":"test.fake","version":"1","contractVersion":1,"name":"Fake","entrypoint":"provider.exe","capabilities":["chat","models","host-login"]}`)
	if err = os.WriteFile(filepath.Join(pkg, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	crypt := func(raw []byte) ([]byte, error) {
		out := append([]byte(nil), raw...)
		for i := range out {
			out[i] ^= 0xaa
		}
		return out, nil
	}
	manager, err := Open(t.TempDir(), crypt, crypt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Install(pkg); err != nil {
		t.Fatal(err)
	}
	if err = manager.Enable("test.fake", true); err != nil {
		t.Fatal(err)
	}
	return manager
}
func TestManagerLifecycle(t *testing.T) {
	m := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, method := range []string{"login/import", "models/refresh"} {
		if _, err := m.Run(ctx, "test.fake", method, nil, nil, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(m.dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private")) {
		t.Fatal("plaintext credential persisted")
	}
	isolated, err := Open(t.TempDir(), m.encrypt, m.decrypt)
	if err != nil || len(isolated.List()) != 0 {
		t.Fatal("environment state leaked")
	}
	if len(m.List()[0].Secret) != 0 {
		t.Fatal("credential exposed")
	}
	catalog := m.Sources()[0].Models[0]
	if catalog.PriceFactor == nil || *catalog.PriceFactor != 0.31 {
		t.Fatal("price factor lost")
	}
	first := catalog.ID
	if _, _, err := m.Resolve(first); err != nil {
		t.Fatal(err)
	}
	reload, err := Open(filepath.Dir(m.dir), m.encrypt, m.decrypt)
	if err != nil {
		t.Fatal(err)
	}
	if reload.Sources()[0].Models[0].ID != first {
		t.Fatal("model identity changed")
	}
	if err = m.Enable("test.fake", false); err != nil {
		t.Fatal(err)
	}
	if _, _, err = m.Resolve(first); err == nil {
		t.Fatal("disabled route resolved")
	}
	if _, err = m.Run(ctx, "test.fake", "models/refresh", nil, nil, "", nil); err == nil {
		t.Fatal("disabled plugin executed")
	}
}
func TestManagerCancelAndCrash(t *testing.T) {
	m := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	if _, err := m.Run(ctx, "test.fake", "wait", nil, nil, "", nil); err == nil {
		t.Fatal("cancellation succeeded")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if _, err := m.Run(ctx2, "test.fake", "crash", nil, nil, "", nil); err == nil {
		t.Fatal("crash succeeded")
	}
	if _, err := m.Run(ctx2, "test.fake", "models/refresh", nil, nil, "", nil); err != nil {
		t.Fatal("restart failed", err)
	}
}

func floatPtr(value float64) *float64 { return &value }

func TestHostLoginCompletesThroughPlugin(t *testing.T) {
	m := fixture(t)
	if _, err := HostLogin(context.Background(), m, "test.missing", func(string) error { return nil }); err == nil || ErrorCode(err) != CodeUnsupported {
		t.Fatalf("unknown plugin accepted: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := HostLogin(ctx, m, "test.fake", func(raw string) error {
		auth, err := url.Parse(raw)
		if err != nil {
			return err
		}
		callback := auth.Query().Get("cb")
		if auth.Query().Get("state") == "" || callback == "" {
			return fmt.Errorf("descriptor placeholders not filled: %s", raw)
		}
		target := callback + "?state=" + url.QueryEscape(auth.Query().Get("state")) + "&token=abc"
		response, err := http.Get(target)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("callback status %d", response.StatusCode)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Account == nil || result.Account.ID != "user-abc" {
		t.Fatalf("account missing: %+v", result)
	}
	if entry := m.List()[0]; len(entry.Accounts) != 1 || entry.Accounts[0].Name != "Host Login" {
		t.Fatalf("account not saved: %+v", entry.Accounts)
	}
}

func versionedPackage(t *testing.T, version string) string {
	t.Helper()
	pkg := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(filepath.Join(pkg, "provider.exe"), os.O_CREATE|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	out.Close()
	raw := []byte(fmt.Sprintf(`{"id":"test.fake","version":%q,"contractVersion":1,"name":"Fake","entrypoint":"provider.exe","capabilities":["chat","models"]}`, version))
	if err = os.WriteFile(filepath.Join(pkg, "manifest.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestPluginRollbackUninstallAndDiagnostics(t *testing.T) {
	m := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := m.Run(ctx, "test.fake", "models/refresh", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	if entry := m.List()[0]; len(entry.PackageHash) != 64 {
		t.Fatalf("missing package hash: %+v", entry)
	}
	if err := m.Enable("test.fake", false); err != nil {
		t.Fatal(err)
	}
	updated, err := m.Update(versionedPackage(t, "2"))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Manifest.Version != "2" || updated.Previous == nil || updated.Previous.Manifest.Version != "1" {
		t.Fatalf("update history missing: %+v", updated)
	}
	rolled, err := m.Rollback("test.fake")
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Manifest.Version != "1" || rolled.Previous == nil || rolled.Previous.Manifest.Version != "2" {
		t.Fatalf("rollback failed: %+v", rolled)
	}
	if err := m.SetDiagnostics("test.fake", true); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable("test.fake", true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run(ctx, "test.fake", "diag", nil, nil, "", nil); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(m.List()[0].DiagnosticsPath)
	if err != nil || !strings.Contains(string(raw), "diagnostic line") {
		t.Fatalf("diagnostics missing: %v %q", err, raw)
	}
	if err := m.Uninstall("test.fake"); err == nil {
		t.Fatal("uninstalled enabled plugin")
	}
	if err := m.Enable("test.fake", false); err != nil {
		t.Fatal(err)
	}
	if err := m.Uninstall("test.fake"); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("state survived uninstall")
	}
	for _, dir := range []string{rolled.PackageDir, rolled.Previous.PackageDir} {
		if _, err := os.Stat(filepath.Join(m.dir, dir)); !os.IsNotExist(err) {
			t.Fatalf("package directory survived: %s %v", dir, err)
		}
	}
}

func TestPackageDigestDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := PackageDigest(dir)
	if err != nil || len(first) != 64 {
		t.Fatalf("digest: %v %q", err, first)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := PackageDigest(dir)
	if err != nil || first == second {
		t.Fatalf("digest did not change: %v %q %q", err, first, second)
	}
}
