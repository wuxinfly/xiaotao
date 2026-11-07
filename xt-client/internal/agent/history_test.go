package agent

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHistoryPagingWithAppendAndEqualTimes(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	at := time.Now().UTC()
	item := &Session{ID: "history_test", Title: "history", UpdatedAt: at}
	for i := 0; i < 241; i++ {
		item.Messages = append(item.Messages, Message{ID: fmt.Sprintf("m%d", i), Role: "user", Content: "hello", CreatedAt: at})
		item.Activity = append(item.Activity, Activity{ID: fmt.Sprintf("a%d", i), StartedAt: at})
	}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	before := ""
	for n := 0; ; n++ {
		page, err := svc.History(item.ID, before)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Session.Messages)+len(page.Session.Activity) > historyPageSize {
			t.Fatal("unbounded page")
		}
		for _, m := range page.Session.Messages {
			if seen[m.ID] {
				t.Fatalf("duplicate %s", m.ID)
			}
			seen[m.ID] = true
		}
		for _, a := range page.Session.Activity {
			if seen[a.ID] {
				t.Fatalf("duplicate %s", a.ID)
			}
			seen[a.ID] = true
		}
		if n == 0 {
			item.Messages = append(item.Messages, Message{ID: "new", CreatedAt: at.Add(time.Second)})
			item.Activity = append(item.Activity, Activity{ID: "new_activity", StartedAt: at.Add(time.Second)})
			if err := svc.saveSession(item); err != nil {
				t.Fatal(err)
			}
		}
		if page.Before == "" {
			break
		}
		if page.Before == before || n > 10 {
			t.Fatal("cursor did not advance")
		}
		before = page.Before
	}
	if len(seen) != 482 || seen["new"] || seen["new_activity"] {
		t.Fatalf("snapshot paging lost or added records: %d", len(seen))
	}
	full, err := svc.Get(item.ID)
	if err != nil || len(full.Messages) != 242 || len(full.Activity) != 242 {
		t.Fatalf("full history changed: %v", err)
	}
	for _, cursor := range []string{"-1:2", "a:b", "1", "1:2:3", "999999999999999999999999:0"} {
		if _, err := svc.History(item.ID, cursor); err == nil {
			t.Fatalf("accepted cursor %s", cursor)
		}
	}
}

func TestHistoryChronologyAndLargeRecord(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	at := time.Now().UTC()
	item := &Session{ID: "large", UpdatedAt: at, Messages: []Message{
		{ID: "old", CreatedAt: at, Content: "old"},
		{ID: "large", CreatedAt: at.Add(time.Second), Content: strings.Repeat("字", historyPageBytes)},
		{ID: "new", CreatedAt: at.Add(3 * time.Second), Content: "new"},
	}, Activity: []Activity{{ID: "between", StartedAt: at.Add(2 * time.Second)}}}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	first, err := svc.History(item.ID, "")
	if err != nil || len(first.Session.Messages) != 1 || first.Session.Messages[0].ID != "new" || len(first.Session.Activity) != 1 {
		t.Fatalf("first page: %+v %v", first, err)
	}
	second, err := svc.History(item.ID, first.Before)
	if err != nil || len(second.Session.Messages) != 1 || second.Session.Messages[0].Content != item.Messages[1].Content {
		t.Fatalf("large record truncated: %v", err)
	}
	third, err := svc.History(item.ID, second.Before)
	if err != nil || len(third.Session.Messages) != 1 || third.Session.Messages[0].ID != "old" || third.Before != "" {
		t.Fatalf("last page: %+v %v", third, err)
	}
	if _, err := svc.History("missing", ""); err == nil {
		t.Fatal("missing session accepted")
	}
}

func TestHistoryEmptyAndOneStream(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	item := &Session{ID: "empty", UpdatedAt: time.Now()}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	page, err := svc.History(item.ID, "")
	if err != nil || page.Before != "" || page.Session.Messages == nil || page.Session.Activity == nil {
		t.Fatalf("empty page: %+v %v", page, err)
	}
	for i := 0; i < 100; i++ {
		item.Messages = append(item.Messages, Message{ID: fmt.Sprint(i), CreatedAt: time.Now()})
	}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	page, err = svc.History(item.ID, "")
	if err != nil || len(page.Session.Messages) != 80 {
		t.Fatalf("one stream: %v", err)
	}
	item.Activity = append(item.Activity, Activity{ID: "later", StartedAt: time.Now()})
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	page, err = svc.History(item.ID, page.Before)
	if err != nil || len(page.Session.Messages) != 20 || len(page.Session.Activity) != 0 || page.Before != "" {
		t.Fatalf("empty stream cursor not frozen: %+v %v", page, err)
	}
}

func TestHistoryKeepsLatestAnswer(t *testing.T) {
	svc := NewService(t.TempDir(), "http://localhost", nil, nil, nil)
	at := time.Now().UTC()
	item := &Session{ID: "latest_answer", UpdatedAt: at, Messages: []Message{{ID: "user", Role: "user", CreatedAt: at}, {ID: "answer", Role: "assistant", CreatedAt: at.Add(200 * time.Second)}}}
	for i := 0; i < 90; i++ {
		item.Activity = append(item.Activity, Activity{ID: fmt.Sprint(i), StartedAt: at.Add(time.Duration(i+1) * time.Second)})
	}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	page, err := svc.History(item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Session.Messages) != 1 || page.Session.Messages[0].ID != "answer" {
		t.Fatal("latest answer missing")
	}
}

func TestHistoryHydratesRunAtPageBoundary(t *testing.T) {
	svc := NewService(t.TempDir(), "unused", nil, nil, nil)
	at := time.Now()
	end := at.Add(time.Hour)
	item := &Session{ID: "boundary", Status: "failed", UpdatedAt: end, RunStats: []RunStats{{RunID: "run", Phase: "failed", StartedAt: at, EndedAt: &end}}}
	for i := 0; i < 100; i++ {
		item.Activity = append(item.Activity, Activity{ID: fmt.Sprint(i), RunID: "run", StartedAt: at.Add(time.Duration(i+1) * time.Second)})
	}
	if err := svc.saveSession(item); err != nil {
		t.Fatal(err)
	}
	page, err := svc.History(item.ID, "")
	if err != nil || len(page.Session.RunStats) != 1 || page.Session.RunStats[0].Phase != "failed" || page.Before == "" {
		t.Fatalf("missing supporting run: %+v %v", page, err)
	}
	seen := map[string]bool{}
	for {
		for _, a := range page.Session.Activity {
			if seen[a.ID] {
				t.Fatal("duplicate activity")
			}
			seen[a.ID] = true
		}
		if page.Before == "" {
			break
		}
		page, err = svc.History(item.ID, page.Before)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 100 {
		t.Fatal("supporting stats shifted cursors")
	}
}
