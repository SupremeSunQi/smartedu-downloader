package lifecycle

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestShutdownCancelsDownloadsFlushesJournalAndClearsSessionInOrder(t *testing.T) {
	calls := &orderedCalls{}
	coordinator := NewCoordinator(
		shutdownFunc(func(context.Context) error { calls.Add("downloads.shutdown"); return nil }),
		clearFunc(func() { calls.Add("session.clear") }),
		flushFunc(func() error { calls.Add("journal.flush"); return nil }),
	)
	if err := coordinator.RequestShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"downloads.shutdown", "journal.flush", "session.clear"}
	if got := calls.Values(); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestShutdownContinuesCleanupAfterErrors(t *testing.T) {
	calls := &orderedCalls{}
	coordinator := NewCoordinator(
		shutdownFunc(func(context.Context) error { calls.Add("downloads.shutdown"); return errors.New("download error") }),
		clearFunc(func() { calls.Add("session.clear") }),
		flushFunc(func() error { calls.Add("journal.flush"); return errors.New("journal error") }),
	)
	err := coordinator.RequestShutdown(context.Background())
	if err == nil || !reflect.DeepEqual(calls.Values(), []string{"downloads.shutdown", "journal.flush", "session.clear"}) {
		t.Fatalf("cleanup stopped early: calls=%v err=%v", calls.Values(), err)
	}
}

type orderedCalls struct {
	mutex sync.Mutex
	items []string
}

func (calls *orderedCalls) Add(value string) {
	calls.mutex.Lock()
	calls.items = append(calls.items, value)
	calls.mutex.Unlock()
}

func (calls *orderedCalls) Values() []string {
	calls.mutex.Lock()
	defer calls.mutex.Unlock()
	return append([]string(nil), calls.items...)
}

type shutdownFunc func(context.Context) error

func (function shutdownFunc) Shutdown(ctx context.Context) error { return function(ctx) }

type clearFunc func()

func (function clearFunc) Clear() { function() }

type flushFunc func() error

func (function flushFunc) Flush() error { return function() }
