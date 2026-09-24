package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalReturnsOnlyRecoverableTasksAndPausesInterruptedWork(t *testing.T) {
	journal := NewTaskJournal(t.TempDir())
	records := []TaskRecord{
		{ID: "one", BookID: "book-1", Filename: "语文.pdf", Status: "downloading", PartialBytes: 12},
		{ID: "two", BookID: "book-2", Filename: "数学.pdf", Status: "completed", PartialBytes: 20},
		{ID: "three", BookID: "book-3", Filename: "英语.pdf", Status: "canceled", PartialBytes: 5},
	}
	if err := journal.Save(records); err != nil {
		t.Fatal(err)
	}
	got, err := journal.LoadForRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("recovery records = %#v, want one incomplete task", got)
	}
	if got[0].Status != "paused" {
		t.Fatalf("incomplete status = %q, want paused", got[0].Status)
	}
}

func TestJournalNeverSerializesResourceURLsOrTokens(t *testing.T) {
	directory := t.TempDir()
	journal := NewTaskJournal(directory)
	if err := journal.Save([]TaskRecord{{ID: "one", BookID: "book-1", Filename: "book.pdf", Status: "paused"}}); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(directory, "downloads.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(payload))
	for _, forbidden := range []string{"accesstoken", "token", "mirror", "http://", "https://"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("journal contains forbidden field %q: %s", forbidden, payload)
		}
	}
}

func TestJournalReconcileReplacesOnlyRecordsOwnedByCurrentManager(t *testing.T) {
	journal := NewTaskJournal(t.TempDir())
	if err := journal.Save([]TaskRecord{
		{ID: "earlier", BookID: "old-book", Filename: "旧任务.pdf", Status: "paused"},
		{ID: "owned", BookID: "book", Filename: "教材.pdf", Status: "downloading"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Reconcile([]string{"owned"}, []TaskRecord{{ID: "owned", BookID: "book", Filename: "教材.pdf", Status: "paused", PartialBytes: 50}}); err != nil {
		t.Fatal(err)
	}
	got, err := journal.LoadForRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "earlier" || got[1].ID != "owned" || got[1].PartialBytes != 50 {
		t.Fatalf("reconciled records = %#v", got)
	}
}

func TestJournalRemoveDeletesOnlySelectedRecoveryRecord(t *testing.T) {
	journal := NewTaskJournal(t.TempDir())
	if err := journal.Save([]TaskRecord{
		{ID: "remove", BookID: "one", Filename: "删除.pdf", Status: "paused"},
		{ID: "keep", BookID: "two", Filename: "保留.pdf", Status: "paused"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := journal.Remove([]string{"remove"}); err != nil {
		t.Fatal(err)
	}
	got, err := journal.LoadForRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("remaining records = %#v, want keep", got)
	}
}

func TestJournalEmptySaveLoadsAsEmptyArray(t *testing.T) {
	journal := NewTaskJournal(t.TempDir())
	if err := journal.Save(nil); err != nil {
		t.Fatal(err)
	}
	got, err := journal.LoadForRecovery()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty recovery records = %#v, want non-nil empty slice", got)
	}
}
