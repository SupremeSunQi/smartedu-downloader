package lifecycle

import (
	"context"
	"errors"
	"sync"
	"time"
)

const shutdownTimeout = 10 * time.Second

type DownloadShutdown interface {
	Shutdown(context.Context) error
}

type SessionClearer interface {
	Clear()
}

type JournalFlusher interface {
	Flush() error
}

type Coordinator struct {
	downloads DownloadShutdown
	session   SessionClearer
	journal   JournalFlusher
	once      sync.Once
	err       error
}

func NewCoordinator(downloads DownloadShutdown, session SessionClearer, journal JournalFlusher) *Coordinator {
	return &Coordinator{downloads: downloads, session: session, journal: journal}
}

func (coordinator *Coordinator) RequestShutdown(ctx context.Context) error {
	coordinator.once.Do(func() {
		shutdownContext, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		var downloadErr, journalErr error
		if coordinator.downloads != nil {
			downloadErr = coordinator.downloads.Shutdown(shutdownContext)
		}
		if coordinator.journal != nil {
			journalErr = coordinator.journal.Flush()
		}
		if coordinator.session != nil {
			coordinator.session.Clear()
		}
		coordinator.err = errors.Join(downloadErr, journalErr)
	})
	return coordinator.err
}
