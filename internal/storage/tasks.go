package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TaskRecord struct {
	ID            string    `json:"id"`
	BookID        string    `json:"bookId"`
	Filename      string    `json:"filename"`
	Status        string    `json:"status"`
	PartialBytes  int64     `json:"partialBytes"`
	ExpectedSize  int64     `json:"expectedSize"`
	MD5           string    `json:"md5,omitempty"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
	FailureCode   string    `json:"failureCode,omitempty"`
	MirrorAttempt int       `json:"mirrorAttempt,omitempty"`
}

type TaskJournal struct {
	directory string
	path      string
	mutex     sync.Mutex
}

// Flush exists for lifecycle coordination; each Save is already synced atomically.
func (journal *TaskJournal) Flush() error {
	return nil
}

func NewTaskJournal(directory string) *TaskJournal {
	return &TaskJournal{directory: directory, path: filepath.Join(directory, "downloads.json")}
}

func (journal *TaskJournal) Save(records []TaskRecord) (err error) {
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	return journal.saveLocked(records)
}

func (journal *TaskJournal) Reconcile(ownedIDs []string, records []TaskRecord) error {
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	existing, err := journal.loadLocked()
	if err != nil {
		return err
	}
	owned := make(map[string]struct{}, len(ownedIDs))
	for _, id := range ownedIDs {
		owned[id] = struct{}{}
	}
	merged := make([]TaskRecord, 0, len(existing)+len(records))
	for _, record := range recoverableRecords(existing) {
		if _, replace := owned[record.ID]; !replace {
			merged = append(merged, record)
		}
	}
	merged = append(merged, recoverableRecords(records)...)
	return journal.saveLocked(merged)
}

func (journal *TaskJournal) Remove(ids []string) error {
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	existing, err := journal.loadLocked()
	if err != nil {
		return err
	}
	removed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		removed[id] = struct{}{}
	}
	remaining := make([]TaskRecord, 0, len(existing))
	for _, record := range recoverableRecords(existing) {
		if _, remove := removed[record.ID]; !remove {
			remaining = append(remaining, record)
		}
	}
	return journal.saveLocked(remaining)
}

func (journal *TaskJournal) saveLocked(records []TaskRecord) (err error) {
	if records == nil {
		records = []TaskRecord{}
	}
	if err := os.MkdirAll(journal.directory, 0o700); err != nil {
		return fmt.Errorf("create task directory: %w", err)
	}
	temporary := journal.path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create task journal: %w", err)
	}
	defer func() {
		closeErr := file.Close()
		removeErr := os.Remove(temporary)
		if errors.Is(closeErr, os.ErrClosed) {
			closeErr = nil
		}
		if errors.Is(removeErr, os.ErrNotExist) {
			removeErr = nil
		}
		err = errors.Join(err, closeErr, removeErr)
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(records); err != nil {
		return fmt.Errorf("encode task journal: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync task journal: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close task journal: %w", err)
	}
	if err := os.Rename(temporary, journal.path); err != nil {
		return fmt.Errorf("replace task journal: %w", err)
	}
	return nil
}

func (journal *TaskJournal) LoadForRecovery() ([]TaskRecord, error) {
	journal.mutex.Lock()
	defer journal.mutex.Unlock()
	records, err := journal.loadLocked()
	if err != nil {
		return nil, err
	}
	return recoverableRecords(records), nil
}

func (journal *TaskJournal) loadLocked() ([]TaskRecord, error) {
	file, err := os.Open(journal.path)
	if errors.Is(err, os.ErrNotExist) {
		return []TaskRecord{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open task journal: %w", err)
	}
	defer file.Close()
	var records []TaskRecord
	decoder := json.NewDecoder(io.LimitReader(file, 8<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode task journal: %w", err)
	}
	if records == nil {
		records = []TaskRecord{}
	}
	return records, nil
}

func recoverableRecords(records []TaskRecord) []TaskRecord {
	recoverable := make([]TaskRecord, 0, len(records))
	for _, record := range records {
		switch record.Status {
		case "queued", "resolving", "downloading":
			record.Status = "paused"
		case "paused", "waiting_auth", "failed":
		default:
			continue
		}
		recoverable = append(recoverable, record)
	}
	return recoverable
}
