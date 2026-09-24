package download

import "time"

type TaskStatus string

const (
	StatusQueued      TaskStatus = "queued"
	StatusResolving   TaskStatus = "resolving"
	StatusDownloading TaskStatus = "downloading"
	StatusPaused      TaskStatus = "paused"
	StatusWaitingAuth TaskStatus = "waiting_auth"
	StatusCompleted   TaskStatus = "completed"
	StatusFailed      TaskStatus = "failed"
	StatusCanceled    TaskStatus = "canceled"
)

type Task struct {
	ID            string     `json:"id"`
	BookID        string     `json:"bookId"`
	Filename      string     `json:"filename"`
	Status        TaskStatus `json:"status"`
	BytesDone     int64      `json:"bytesDone"`
	TotalBytes    int64      `json:"totalBytes"`
	SpeedBytes    int64      `json:"speedBytes"`
	ETASeconds    int64      `json:"etaSeconds"`
	MirrorAttempt int        `json:"mirrorAttempt"`
	Error         string     `json:"error,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type TaskEvent struct {
	Task Task `json:"task"`
}
