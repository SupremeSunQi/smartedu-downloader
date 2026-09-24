package catalog

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const (
	dimensionStage   = "zxxxd"
	dimensionSubject = "zxxxk"
	dimensionEdition = "zxxbb"
	dimensionGrade   = "zxxnj"
	dimensionVolume  = "zxxcc"
)

type Textbook struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Provider     string `json:"provider"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Size         int64  `json:"size"`
	StageID      string `json:"stageId,omitempty"`
	Stage        string `json:"stage,omitempty"`
	SubjectID    string `json:"subjectId,omitempty"`
	Subject      string `json:"subject,omitempty"`
	EditionID    string `json:"editionId,omitempty"`
	Edition      string `json:"edition,omitempty"`
	GradeID      string `json:"gradeId,omitempty"`
	Grade        string `json:"grade,omitempty"`
	VolumeID     string `json:"volumeId,omitempty"`
	Volume       string `json:"volume,omitempty"`
}

type TagOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Filters struct {
	Stages   []TagOption `json:"stages"`
	Subjects []TagOption `json:"subjects"`
	Editions []TagOption `json:"editions"`
	Grades   []TagOption `json:"grades"`
	Volumes  []TagOption `json:"volumes"`
}

type Snapshot struct {
	Version    int64           `json:"version"`
	SyncedAt   time.Time       `json:"syncedAt"`
	Textbooks  []Textbook      `json:"textbooks"`
	Filters    Filters         `json:"filters"`
	SourceTags json.RawMessage `json:"sourceTags,omitempty"`
}

type Query struct {
	Search    string `json:"search"`
	StageID   string `json:"stageId"`
	SubjectID string `json:"subjectId"`
	EditionID string `json:"editionId"`
	GradeID   string `json:"gradeId"`
	VolumeID  string `json:"volumeId"`
}

func Filter(snapshot Snapshot, query Query) []Textbook {
	search := strings.ToLower(strings.TrimSpace(query.Search))
	result := make([]Textbook, 0, len(snapshot.Textbooks))
	for _, book := range snapshot.Textbooks {
		if query.StageID != "" && book.StageID != query.StageID ||
			query.SubjectID != "" && book.SubjectID != query.SubjectID ||
			query.EditionID != "" && book.EditionID != query.EditionID ||
			query.GradeID != "" && book.GradeID != query.GradeID ||
			query.VolumeID != "" && book.VolumeID != query.VolumeID {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(book.Title + "\n" + book.Provider)
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		result = append(result, book)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Title == result[j].Title {
			return result[i].ID < result[j].ID
		}
		return result[i].Title < result[j].Title
	})
	return result
}
