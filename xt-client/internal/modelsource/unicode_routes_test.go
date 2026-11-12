package modelsource

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestCallIDKeepsUnicodeWithoutDecodingStructuralEscapes(t *testing.T) {
	cases := []struct{ source, key, want string }{
		{"我的接口", "中文/模型%", "我的接口/中文%2F模型%25"},
		{"我的接口", "%E4%B8%AD", "我的接口/%25E4%25B8%25AD"},
		{"a/b", "中 文+", "a%2Fb/中%20文%2B"},
		{"a", "中\u3000文\n", "a/中%E3%80%80文%0A"},
	}
	for _, tc := range cases {
		if got := CallID(tc.source, tc.key); got != tc.want {
			t.Fatalf("%q: got %q, want %q", tc.key, got, tc.want)
		}
		if got := ReadableCallID(tc.want); got != tc.want {
			t.Fatalf("normalization is not idempotent: %s", got)
		}
	}
}

func TestEncodedChineseAliasesSurviveUpgradeAndKeepTombstones(t *testing.T) {
	enc, dec := testCrypto(t)
	dir := t.TempDir()
	s, _ := Open(dir, enc, dec)
	in := input()
	in.Label, in.Models[0].UpstreamModelID = "我的接口", "中文/模型"
	src, err := s.Save("", in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.path)
	var disk diskFile
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	old := legacyCallID("我的接口", "中文/模型")
	disk.Routes[src.Models[0].ID] = old
	raw, _ = json.Marshal(disk)
	if err := os.WriteFile(s.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	want := "我的接口/中文%2F模型"
	if s.List()[0].Models[0].CallID != want {
		t.Fatal(s.List())
	}
	for _, id := range []string{old, want, src.Models[0].ID} {
		_, model, _, err := s.Resolve(id)
		if err != nil || model.ID != src.Models[0].ID {
			t.Fatal(id, model, err)
		}
	}
	in.Models = nil
	if _, err = s.Save(src.ID, in); err != nil {
		t.Fatal(err)
	}
	in.Models = []Model{{UpstreamModelID: "中文/模型"}}
	replacement, err := s.Save(src.ID, in)
	if err != nil || replacement.Models[0].CallID != want+"~2" {
		t.Fatal(replacement, err)
	}
	for _, id := range []string{old, want} {
		if _, _, _, err := s.Resolve(id); !errors.Is(err, ErrNotFound) {
			t.Fatal("deleted alias rebound", err)
		}
	}
}

func TestPluginPrefixReservationSurvivesRestartAndDeletion(t *testing.T) {
	enc, dec := testCrypto(t)
	dir := t.TempDir()
	s, _ := Open(dir, enc, dec)
	prefix, err := s.PluginPrefix("xiaotao.trae")
	if err != nil || prefix != "trae" {
		t.Fatal(prefix, err)
	}
	s, err = Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	in := input()
	in.Label = "Trae"
	src, err := s.Save("", in)
	if err != nil || src.Models[0].CallID != "trae~2/same-upstream" {
		t.Fatal(src, err)
	}
	if err := s.Delete(src.ID); err != nil {
		t.Fatal(err)
	}
	prefix, err = s.PluginPrefix("xiaotao.trae")
	if err != nil || prefix != "trae" {
		t.Fatal(prefix, err)
	}
}

func TestFailedPluginPrefixWriteDoesNotReserveName(t *testing.T) {
	s := openTest(t)
	original := s.path
	s.path = t.TempDir()
	if _, err := s.PluginPrefix("xiaotao.trae"); err == nil {
		t.Fatal("expected failed prefix persistence")
	}
	s.path = original
	in := input()
	in.Label = "Trae"
	src, err := s.Save("", in)
	if err != nil || src.Models[0].CallID != "trae/same-upstream" {
		t.Fatal(src, err)
	}
}

func TestImportCannotOverwriteReservedPluginPrefix(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "merge", true: "conflict"}[conflict], func(t *testing.T) {
			enc, dec := testCrypto(t)
			sourceDir := t.TempDir()
			source, _ := Open(sourceDir, enc, dec)
			in := input()
			in.Label = "Trae"
			if _, err := source.Save("", in); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(source.path)
			target, _ := Open(t.TempDir(), enc, dec)
			plugin := "xiaotao.other"
			if conflict {
				plugin = "xiaotao.trae"
			}
			prefix, err := target.PluginPrefix(plugin)
			if err != nil {
				t.Fatal(err)
			}
			_, err = target.ImportFrom(sourceDir, dec)
			if conflict && err == nil {
				t.Fatal("import took over plugin prefix")
			}
			if !conflict && err != nil {
				t.Fatal(err)
			}
			if conflict && len(target.List()) != 0 {
				t.Fatal("failed import changed target sources")
			}
			if target.routes["plugin:"+plugin] != prefix {
				t.Fatal("import changed reserved prefix")
			}
			after, _ := os.ReadFile(source.path)
			if string(before) != string(after) {
				t.Fatal("import modified production source")
			}
		})
	}
}
