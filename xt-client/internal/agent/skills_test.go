package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTestSkill(t *testing.T, root, dir, name, description string) string {
	t.Helper()
	file := filepath.Join(root, dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("---\nname: "+name+"\ndescription: "+description+"\n---\n\nSECRET_SKILL_BODY_NOT_PRELOADED"), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestLocalSkillsDiscoveryAndConflicts(t *testing.T) {
	root := t.TempDir()
	writeTestSkill(t, root, "first", "Review", ">-\n  Review the code.\n  Report evidence.")
	writeTestSkill(t, root, "second", "review", "Another reviewer.")
	writeTestSkill(t, root, "core", "xiaotao", "Cannot replace Core.")
	writeTestSkill(t, root, "nested/deeper", "hidden", "Not a direct child.")
	settings := SkillsSettings{Directories: []SkillDirectory{{Path: root}}}
	catalog, err := ScanLocalSkills(settings)
	if err != nil || len(catalog.Skills) != 3 {
		t.Fatalf("catalog: %+v %v", catalog, err)
	}
	var firstID, secondID string
	for _, skill := range catalog.Skills {
		if skill.Enabled || skill.Error == "" {
			t.Fatalf("conflict loaded: %+v", skill)
		}
		if skill.Name == "Review" {
			firstID = skill.ID
			if skill.Description != "Review the code. Report evidence." {
				t.Fatal(skill.Description)
			}
		}
		if skill.Name == "review" {
			secondID = skill.ID
		}
	}
	settings.DisabledSkills = []string{secondID}
	catalog, _ = ScanLocalSkills(settings)
	for _, skill := range catalog.Skills {
		if skill.ID == firstID && (!skill.Enabled || skill.Error != "") {
			t.Fatal("disabling duplicate did not resolve conflict")
		}
	}
	// A direct skill directory and its parent must not duplicate the same file.
	settings.Directories = append(settings.Directories, SkillDirectory{Path: filepath.Join(root, "first")})
	catalog, _ = ScanLocalSkills(settings)
	if len(catalog.Skills) != 3 {
		t.Fatal("overlapping roots duplicated skills")
	}
}

func TestLocalSkillsMetadataErrorsAreVisibleAndBounded(t *testing.T) {
	root := t.TempDir()
	for i, content := range []string{
		"---\n---", "no metadata", "---\nname: one\nname: two\ndescription: duplicate\n---\n",
		"---\nname: 12\ndescription: number\n---\n", "---\nname: okay\n---\n",
		"---\nname: bad\ndescription: [a, b]\n---\n", string([]byte{0xff, 0xfe}), strings.Repeat("x", maxSkillBytes+1),
	} {
		file := writeTestSkill(t, root, fmt.Sprint(i), "sample", "sample")
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	catalog, err := ScanLocalSkills(SkillsSettings{Directories: []SkillDirectory{{Path: root}, {Path: filepath.Join(root, "missing")}}})
	if err != nil || len(catalog.Skills) != 0 || len(catalog.Issues) != 9 {
		t.Fatalf("errors swallowed: %+v %v", catalog, err)
	}
	for i := 0; i < 140; i++ {
		writeTestSkill(t, root, fmt.Sprintf("valid-%03d", i), fmt.Sprintf("skill-%d", i), "valid")
	}
	catalog, err = ScanLocalSkills(SkillsSettings{Directories: []SkillDirectory{{Path: root}}})
	if err != nil || len(catalog.Skills) != maxLocalSkills || len(catalog.Issues) == 0 {
		t.Fatalf("limits: %d %v", len(catalog.Skills), err)
	}
}

func TestSkillsSettingsIsolationAndInvalidSave(t *testing.T) {
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	root := t.TempDir()
	writeTestSkill(t, root, "", "local", "A local skill.")
	settings := SkillsSettings{Directories: []SkillDirectory{{Path: root}}}
	if err := svc.SaveSkillsSettings(settings); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []SkillsSettings{
		{Directories: []SkillDirectory{{Path: "relative"}}},
		{Directories: []SkillDirectory{{Path: root}, {Path: root}}},
		{DisabledSkills: []string{"not-a-skill-id"}},
	} {
		if err := svc.SaveSkillsSettings(bad); err == nil {
			t.Fatal("bad settings saved")
		}
	}
	catalog, err := svc.LocalSkills()
	if err != nil || len(catalog.Skills) != 1 || !catalog.Skills[0].Enabled {
		t.Fatalf("saved config lost: %+v %v", catalog, err)
	}
	other := NewService(t.TempDir(), "", nil, nil, nil)
	empty, _ := other.LocalSkills()
	if len(empty.Skills) != 0 || len(empty.Settings.Directories) != 0 {
		t.Fatal("environments share skills")
	}
	settings.Directories[0].Disabled = true
	if err := svc.SaveSkillsSettings(settings); err != nil {
		t.Fatal(err)
	}
	catalog, _ = svc.LocalSkills()
	if len(catalog.Skills) != 0 {
		t.Fatal("disabled root scanned")
	}
	if err := os.WriteFile(filepath.Join(svc.dataDir, "skills.json"), []byte(`{"directories":[]} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SkillsSettings(); err == nil {
		t.Fatal("corrupt config silently ignored")
	}
}

func TestSkillFileLinksCannotEscapeSelectedRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	target := writeTestSkill(t, outside, "", "outside", "Outside selected root.")
	if err := os.Symlink(target, filepath.Join(root, "SKILL.md")); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	catalog, err := ScanLocalSkills(SkillsSettings{Directories: []SkillDirectory{{Path: root}}})
	if err != nil || len(catalog.Skills) != 0 || len(catalog.Issues) != 1 {
		t.Fatalf("escaped link: %+v %v", catalog, err)
	}
}

func TestSkillDirectoryJunctionCannotEscapeSelectedRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows directory junction regression")
	}
	root, outside := t.TempDir(), t.TempDir()
	writeTestSkill(t, outside, "", "outside", "Outside selected root.")
	command := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(root, "linked"), outside)
	hideCommandWindow(command)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create isolated junction: %v %s", err, output)
	}
	catalog, err := ScanLocalSkills(SkillsSettings{Directories: []SkillDirectory{{Path: root}}})
	if err != nil || len(catalog.Skills) != 0 || len(catalog.Issues) != 1 || catalog.Issues[0].Message == "" {
		t.Fatalf("junction escaped discovery: %+v %v", catalog, err)
	}
}

func TestSendProvidesOnlyEnabledMetadataAndKeepsCore(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		prompts = append(prompts, body.Messages[0].Content)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"done"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, func() string { return "test" }, server.Client(), nil)
	root := t.TempDir()
	file := writeTestSkill(t, root, "", "review", "Review code when requested.")
	settings := SkillsSettings{Directories: []SkillDirectory{{Path: root}}}
	if err := svc.SaveSkillsSettings(settings); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Send(context.Background(), session.ID, "use review", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompts[0], "Review code when requested.") || !strings.Contains(prompts[0], "以下是完整 XiaoTao SKILL.md") || strings.Contains(prompts[0], "SECRET_SKILL_BODY") || !strings.Contains(prompts[0], "不改变当前访问模式") {
		t.Fatal("initial prompt lost Core or preloaded body")
	}
	catalog, _ := svc.LocalSkills()
	settings.DisabledSkills = []string{catalog.Skills[0].ID}
	_ = svc.SaveSkillsSettings(settings)
	if err = svc.Send(context.Background(), session.ID, "again", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompts[1], "Review code when requested.") || !strings.Contains(prompts[1], "持续守卫") {
		t.Fatal("disabled skill remained or persona lost")
	}
	settings.DisabledSkills = nil
	_ = svc.SaveSkillsSettings(settings)
	_ = os.Remove(file)
	if err = svc.Send(context.Background(), session.ID, "again", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompts[2], "未找到 SKILL.md") || strings.Contains(prompts[2], "Review code when requested.") {
		t.Fatal("missing skill was still advertised")
	}
	if err := svc.SaveSkillsSettings(SkillsSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Send(context.Background(), session.ID, "again", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompts[3], "不从旧会话记录恢复已移除技能") {
		t.Fatal("removing all skills did not revoke the old catalog")
	}
}

func TestSkillBodyIsReadThroughExistingToolAfterDiscovery(t *testing.T) {
	root := t.TempDir()
	file := writeTestSkill(t, root, "", "review", "Review current changes.")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role       string `json:"role"`
				Content    string `json:"content"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			if strings.Contains(body.Messages[0].Content, "SECRET_SKILL_BODY") {
				t.Error("body preloaded before tool call")
			}
			arguments, _ := json.Marshal(map[string]any{"path": file})
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": "", "tool_calls": []any{map[string]any{"id": "read-skill", "type": "function", "function": map[string]any{"name": "read_file", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
			return
		}
		last := body.Messages[len(body.Messages)-1]
		if last.Role != "tool" || last.ToolCallID != "read-skill" || !strings.Contains(last.Content, "SECRET_SKILL_BODY_NOT_PRELOADED") {
			t.Errorf("skill read result not returned: %+v", last)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"reviewed"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, func() string { return "test" }, server.Client(), nil)
	if err := svc.SaveSkillsSettings(SkillsSettings{Directories: []SkillDirectory{{Path: root}}}); err != nil {
		t.Fatal(err)
	}
	session, err := svc.Create(Options{Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Send(context.Background(), session.ID, "use review", func(Event) {}); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("expected discovery then read tool result, got %d requests", requests)
	}
}

func TestSkillPromptBudgetReportsOmissions(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxLocalSkills; i++ {
		writeTestSkill(t, root, fmt.Sprintf("%03d", i), fmt.Sprintf("skill-%03d", i), strings.Repeat("界", 1024))
	}
	svc := NewService(t.TempDir(), "", nil, nil, nil)
	if err := svc.SaveSkillsSettings(SkillsSettings{Directories: []SkillDirectory{{Path: root}}}); err != nil {
		t.Fatal(err)
	}
	prompt, err := svc.localSkillsInstructions()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(prompt, "本地技能索引（JSON 数据）：\n")
	if len(parts) != 2 || len(parts[1]) > 64*1024+1 {
		t.Fatal("skill index exceeded prompt budget")
	}
	var data struct {
		Omitted int `json:"skills_omitted"`
	}
	if err := json.Unmarshal([]byte(parts[1]), &data); err != nil || data.Omitted == 0 {
		t.Fatalf("omissions not reported: %v %+v", err, data)
	}
}
