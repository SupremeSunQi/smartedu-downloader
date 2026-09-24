package catalog

import "testing"

func TestFilterAppliesAllSelectedDimensionsAndSearch(t *testing.T) {
	snapshot := sampleSnapshot()
	got := Filter(snapshot, Query{Search: "语文", StageID: "primary", GradeID: "grade-1"})
	if len(got) != 1 || got[0].ID != "book-1" {
		t.Fatalf("Filter returned %#v, want book-1", got)
	}
}

func TestFilterSearchesTitleAndProviderCaseInsensitively(t *testing.T) {
	snapshot := sampleSnapshot()
	for _, search := range []string{"ENGLISH", "press"} {
		got := Filter(snapshot, Query{Search: search})
		if len(got) != 1 || got[0].ID != "book-2" {
			t.Errorf("search %q returned %#v, want book-2", search, got)
		}
	}
}

func TestFilterReturnsStableTitleOrder(t *testing.T) {
	got := Filter(sampleSnapshot(), Query{})
	if len(got) != 3 {
		t.Fatalf("Filter returned %d books, want 3", len(got))
	}
	want := []string{"book-2", "book-1", "book-3"}
	for index, id := range want {
		if got[index].ID != id {
			t.Fatalf("result %d ID = %q, want %q", index, got[index].ID, id)
		}
	}
}

func sampleSnapshot() Snapshot {
	return Snapshot{Textbooks: []Textbook{
		{ID: "book-1", Title: "语文一年级上册", Provider: "人民教育出版社", StageID: "primary", GradeID: "grade-1"},
		{ID: "book-2", Title: "English 二年级", Provider: "Sample Press", StageID: "primary", GradeID: "grade-2"},
		{ID: "book-3", Title: "道德与法治", Provider: "人民教育出版社", StageID: "primary", GradeID: "grade-1"},
	}}
}
