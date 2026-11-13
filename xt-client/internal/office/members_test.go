package office

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xiaotao/xt-client/internal/agent"
)

func TestBootstrapCRUDInvitePromoteAndHistoryIndependent(t *testing.T) {
	store := NewRegistryStore(t.TempDir())
	service := NewTeamService(store)
	project, second := t.TempDir(), t.TempDir()
	empty, err := service.Snapshot(project)
	if err != nil {
		t.Fatal(err)
	}
	drafts := TeamTemplates()[0].Roles
	created, err := service.Bootstrap(project, drafts, revisions(empty))
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Members) != 3 || len(created.GlobalRoles) != 0 {
		t.Fatalf("template leaked: %+v", created)
	}
	for _, member := range created.Members {
		if member.Scope != RoleScopeProject || !roleIDPattern.MatchString(member.ID) {
			t.Fatal("unstable template identity")
		}
	}
	draft := drafts[0]
	draft.Name = "全局产品"
	withGlobal, err := service.CreateRole(project, RoleScopeGlobal, draft, revisions(created))
	if err != nil {
		t.Fatal(err)
	}
	globalID := withGlobal.GlobalRoles[0].ID
	secondBefore, err := service.Snapshot(second)
	if err != nil {
		t.Fatal(err)
	}
	invited, err := service.Invite(second, globalID, revisions(secondBefore))
	if err != nil {
		t.Fatal(err)
	}
	if invited.Members[0].Name != "全局产品" || invited.Members[0].Scope != RoleScopeGlobal {
		t.Fatal("configuration not reused")
	}
	other, err := service.Snapshot(project)
	if err != nil || len(other.Members) != 3 {
		t.Fatal("cross-project membership leaked")
	}
	promoted, err := service.Promote(project, created.Members[0].ID, revisions(other))
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Members[0].Scope != RoleScopeGlobal || promoted.Members[0].ID != created.Members[0].ID {
		t.Fatal("promotion identity changed")
	}
	removed, err := service.Remove(project, created.Members[0].ID, revisions(promoted))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = removed.Member(created.Members[0].ID); err == nil {
		t.Fatal("removed member remains active")
	}
	if len(storePaths(store, project)) != 3 {
		t.Fatal("registry files missing")
	}
	edited, err := service.UpdateRole(project, created.Members[1].ID, RoleDraft{Name: strings.Repeat("设", 40), Responsibility: "更新", Capabilities: []string{"设计"}, Appearance: DraftAppearance(1)}, revisions(removed))
	if err != nil || edited.Members[0].ID != created.Members[1].ID {
		t.Fatalf("edit identity: %v", err)
	}
	archived, err := service.Remove(project, created.Members[1].ID, revisions(edited))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Archive(project, created.Members[1].ID, revisions(archived)); err != nil {
		t.Fatal(err)
	}
}

func TestMemberServiceKeepsConversationsAndTasksIndependent(t *testing.T) {
	exec := newFakeExecutor()
	svc, project := newTestService(t, exec)
	empty, err := svc.teams.Snapshot(project)
	if err != nil {
		t.Fatal(err)
	}
	team, err := svc.teams.Bootstrap(project, TeamTemplates()[0].Roles[:1], revisions(empty))
	if err != nil {
		t.Fatal(err)
	}
	roleID := team.Members[0].ID
	if _, err = svc.SendChat(project, "designer", ChatInput{Content: "旧成员", Model: "test/model"}); err == nil {
		t.Fatal("fixed member remains active")
	}
	if _, err = svc.SendChat(project, roleID, ChatInput{Content: "你好"}); err == nil || !strings.Contains(err.Error(), "不可用") {
		t.Fatalf("missing model should fail: %v", err)
	}
	team.Members[0].Model.ModelID = "test/model"
	updated, err := svc.teams.UpdateRole(project, roleID, RoleDraft{Name: team.Members[0].Name, Responsibility: team.Members[0].Responsibility, Capabilities: team.Members[0].Capabilities, Model: team.Members[0].Model, Appearance: team.Members[0].Appearance}, revisions(team))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SendChat(project, roleID, ChatInput{Content: "你好"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		chat, chatErr := svc.Chat(project, roleID)
		return chatErr == nil && chat.Status == StatusCompleted
	})
	removed, err := svc.teams.Remove(project, roleID, revisions(updated))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.SendChat(project, roleID, ChatInput{Content: "不应发送"}); err == nil {
		t.Fatal("removed member sent chat")
	}
	history, err := svc.ChatHistory(project, roleID, "")
	if err != nil || history == nil {
		t.Fatalf("history removed: %v", err)
	}
	snapshot, err := svc.Snapshot(project, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range snapshot.Members {
		if member.ID == roleID {
			t.Fatal("removed member still visible")
		}
	}
	if len(removed.Members) != 0 {
		t.Fatal("team not empty")
	}
}

func revisions(snapshot TeamSnapshot) ProjectRegistries {
	return ProjectRegistries{Global: RoleRegistry{Revision: snapshot.GlobalRevision}, Project: RoleRegistry{Revision: snapshot.ProjectRevision}, Team: TeamRegistry{Revision: snapshot.TeamRevision}}
}

func storePaths(store *RegistryStore, project string) []string {
	paths := store.paths(project)
	return []string{paths.global, paths.project, paths.team}
}

func TestActiveRemovalGuardAndIncompleteArchiveIndex(t *testing.T) {
	exec := newFakeExecutor()
	started := make(chan struct{})
	exec.run = func(ctx context.Context, _, _ string, _ func(agent.Event)) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	svc, project := newTestService(t, exec)
	ids := makeTeamForChat(t, svc, project)
	team, _ := svc.teams.Snapshot(project)
	if _, err := svc.SendChat(project, ids[0], ChatInput{Content: "运行"}); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := svc.teams.Remove(project, ids[0], revisions(team)); !errors.Is(err, ErrMemberActive) {
		t.Fatalf("active guard: %v", err)
	}
	if err := svc.CancelChat(project, ids[0]); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		chat, err := svc.Chat(project, ids[0])
		return err == nil && chat.Status == StatusCanceled
	})
	globalTeam, err := svc.teams.CreateRole(project, RoleScopeGlobal, TeamTemplates()[0].Roles[0], revisions(team))
	if err != nil {
		t.Fatal(err)
	}
	cleanStore := NewRegistryStore(t.TempDir())
	cleanService := NewTeamService(cleanStore)
	cleanSnapshot, err := cleanService.CreateRole(t.TempDir(), RoleScopeGlobal, TeamTemplates()[0].Roles[0], ProjectRegistries{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cleanService.Archive(t.TempDir(), cleanSnapshot.GlobalRoles[0].ID, revisions(cleanSnapshot)); err == nil {
		t.Fatal("archive without complete index succeeded")
	}
	if _, err = svc.teams.Archive(project, ids[2], revisions(globalTeam)); err == nil {
		t.Fatal("member archive succeeded")
	}
	if len(exec.created) == 0 || exec.created[0].Role == "worker" {
		t.Fatal("chat replaced task executor")
	}
}
