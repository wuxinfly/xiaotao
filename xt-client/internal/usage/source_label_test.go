package usage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestSourceLabelSnapshotAndRestart(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "fresh"
		if legacy {
			name = "legacy"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if legacy {
				createLegacySourceLabelDB(t, dir)
			}
			svc, err := New(dir, "test")
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			t.Cleanup(func() {
				if !closed {
					_ = svc.Close()
				}
			})
			req := svc.StartRequest("source-label", "plugin", "model", "xiaotao.codeagent", "", "")
			req.SetSourceLabel("CodeAgent")
			req.SetSourceLabel("")
			snap, err := svc.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.ActiveRequests) != 1 || snap.ActiveRequests[0].SourceLabel != "CodeAgent" {
				t.Fatalf("source missing from active snapshot: %+v", snap.ActiveRequests)
			}
			if snap.ActiveRequests[0].ActionTitle != "对话交互" {
				t.Fatalf("unexpected default action: %+v", snap.ActiveRequests[0])
			}
			req.OnCompleted()
			waitForSourceLabelRecord(t, svc.db)
			if err := svc.Close(); err != nil {
				t.Fatal(err)
			}
			closed = true

			reopened, err := New(dir, "test")
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			records, err := reopened.db.LoadRecentRecords(10)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].SourceLabel != "CodeAgent" || records[0].AccountID != "xiaotao.codeagent" {
				t.Fatalf("source identity not restored: %+v", records)
			}
			snap, err = reopened.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.RecentRequests) != 1 || snap.RecentRequests[0].SourceLabel != "CodeAgent" {
				t.Fatalf("source missing from restored snapshot: %+v", snap.RecentRequests)
			}
		})
	}
}

// Seed the pre-source_label schema so New must migrate it before writing.
func createLegacySourceLabelDB(t *testing.T, dir string) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE request_records (
		request_id TEXT PRIMARY KEY, batch_id TEXT NOT NULL,
		channel TEXT NOT NULL DEFAULT 'trae', model TEXT NOT NULL,
		account_id TEXT NOT NULL, action_tag TEXT DEFAULT '',
		action_title TEXT DEFAULT '', created_at INTEGER NOT NULL,
		first_token_at INTEGER, ended_at INTEGER, status TEXT NOT NULL,
		prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER,
		error_summary TEXT, is_finalized INTEGER NOT NULL DEFAULT 0
	)`)
	if err != nil {
		t.Fatal(err)
	}
}

func waitForSourceLabelRecord(t *testing.T, db *DB) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		records, err := db.LoadRecentRecords(10)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) == 1 && records[0].Status == StatusCompleted && records[0].SourceLabel == "CodeAgent" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("source record was not persisted: %+v", records)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSourceLabelUpsertPreservesKnownLabel(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	rec := &Record{
		RequestID: "label-upsert", BatchID: "test", Channel: "plugin",
		Model: "model", AccountID: "source-id", SourceLabel: "CodeAgent",
		CreatedAt: now, Status: StatusPending,
	}
	if err := db.CommitRequestAndUsage(rec, DateInShanghai(now), 0, 0, 0, false, ""); err != nil {
		t.Fatal(err)
	}
	rec.SourceLabel = ""
	if err := db.CommitRequestAndUsage(rec, DateInShanghai(now), 0, 0, 0, false, ""); err != nil {
		t.Fatal(err)
	}
	records, err := db.LoadRecentRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].SourceLabel != "CodeAgent" {
		t.Fatalf("empty upsert erased source label: %+v", records)
	}
}
