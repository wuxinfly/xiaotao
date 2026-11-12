package modelsource

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestReadableRoutesSurviveEditsRestartAndDeletion(t *testing.T) {
	enc, dec := testCrypto(t)
	dir := t.TempDir()
	s, err := Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	in := input()
	in.Label = "My API"
	in.Models[0].UpstreamModelID = "org/model /+%"
	first, err := s.Save("", in)
	if err != nil {
		t.Fatal(err)
	}
	id := first.Models[0].CallID
	if id != "my-api/org%2Fmodel%20%2F%2B%25" {
		t.Fatal(id)
	}
	second, err := s.Save("", in)
	if err != nil || second.Models[0].CallID != "my-api~2/org%2Fmodel%20%2F%2B%25" {
		t.Fatal(second, err)
	}
	in.Label = "Renamed"
	in.Models = first.Models
	in.Models[0].UpstreamModelID = "changed"
	in.Models[0].CallID = "malicious/override"
	edited, err := s.Save(first.ID, in)
	if err != nil || edited.Models[0].CallID != id {
		t.Fatal(edited, err)
	}
	s, err = Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{id, first.Models[0].ID} {
		source, model, _, err := s.Resolve(route)
		if err != nil || source.ID != first.ID || model.UpstreamModelID != "changed" {
			t.Fatal(source, model, err)
		}
	}
	disabled := false
	in.Enabled = &disabled
	if _, err = s.Save(first.ID, in); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = s.Resolve(id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err = s.Delete(first.ID); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	in = input()
	in.Label = "My API"
	recreated, err := s.Save("", in)
	if err != nil || recreated.Models[0].CallID != "my-api~3/same-upstream" {
		t.Fatal(recreated, err)
	}
	if _, _, _, err = s.Resolve(id); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted alias rebound", err)
	}
}

func TestLegacyMigrationAndReadOnlyImport(t *testing.T) {
	enc, dec := testCrypto(t)
	dir := t.TempDir()
	s, _ := Open(dir, enc, dec)
	source, err := s.Save("", input())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(s.path)
	var disk diskFile
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	disk.Routes = nil
	raw, _ = json.Marshal(disk)
	if err := os.WriteFile(s.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	target, _ := Open(t.TempDir(), enc, dec)
	if _, err := target.ImportFrom(dir, dec); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.path)
	if string(raw) != string(after) {
		t.Fatal("import modified production file")
	}
	reopened, err := Open(dir, enc, dec)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(target.List(), reopened.List()) {
		t.Fatal("migration/import disagree")
	}
	if _, _, _, err := reopened.Resolve(source.Models[0].ID); err != nil {
		t.Fatal(err)
	}
	if reopened.List()[0].Models[0].CallID != "local/same-upstream" {
		t.Fatal(reopened.List())
	}
}

func TestDeletedModelAliasIsNotReused(t *testing.T) {
	s := openTest(t)
	in := input()
	source, err := s.Save("", in)
	if err != nil {
		t.Fatal(err)
	}
	old := source.Models[0].CallID
	in.Models = nil
	if _, err = s.Save(source.ID, in); err != nil {
		t.Fatal(err)
	}
	in = input()
	replacement, err := s.Save(source.ID, in)
	if err != nil || replacement.Models[0].CallID != old+"~2" {
		t.Fatal(replacement, err)
	}
	if _, _, _, err = s.Resolve(old); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted model rebound", err)
	}
}

func TestCorruptCallRoutesFailWithoutOverwriting(t *testing.T) {
	for _, mode := range []string{"duplicate", "wrong-source", "plugin-prefix"} {
		t.Run(mode, func(t *testing.T) {
			enc, dec := testCrypto(t)
			dir := t.TempDir()
			s, _ := Open(dir, enc, dec)
			src, err := s.Save("", input())
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := os.ReadFile(s.path)
			var disk diskFile
			if err := json.Unmarshal(raw, &disk); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "duplicate":
				disk.Routes["model_other"] = disk.Routes[src.Models[0].ID]
			case "wrong-source":
				disk.Routes[src.Models[0].ID] = "someone-else/model"
			case "plugin-prefix":
				disk.Routes[src.ID] = "test.plugin"
			}
			raw, _ = json.Marshal(disk)
			if err := os.WriteFile(s.path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(dir, enc, dec); err == nil {
				t.Fatal("corrupt alias accepted")
			}
			after, _ := os.ReadFile(s.path)
			if string(after) != string(raw) {
				t.Fatal("damaged file overwritten")
			}
		})
	}
}
