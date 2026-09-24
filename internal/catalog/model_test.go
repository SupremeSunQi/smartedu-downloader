package catalog

import (
	"encoding/json"
	"testing"
)

func TestSnapshotSerializesSourceTagsAsJSON(t *testing.T) {
	payload, err := json.Marshal(Snapshot{Version: 1, Textbooks: []Textbook{}, SourceTags: []byte(`{"tag_path":"root"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatal(err)
	}
	if _, ok := document["sourceTags"].(map[string]any); !ok {
		t.Fatalf("sourceTags serialized as %T, want JSON object: %s", document["sourceTags"], payload)
	}
}
