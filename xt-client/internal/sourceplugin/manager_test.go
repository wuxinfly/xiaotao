package sourceplugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
		case "models/refresh":
			factor := 0.31
			result = Result{Models: []Model{{Key: "stable", UpstreamID: "org/model", Label: "Example", PriceFactor: &factor}}}
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
	raw := []byte(`{"id":"test.fake","version":"1","contractVersion":1,"name":"Fake","entrypoint":"provider.exe","capabilities":["chat","models"]}`)
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
