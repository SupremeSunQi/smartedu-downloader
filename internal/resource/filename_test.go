package resource

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSafeFilenameNeutralizesWindowsReservedNamesAndTraversal(t *testing.T) {
	tests := map[string]string{
		`..\CON.`:     "_CON.pdf",
		`语文:一年级`:      "语文_一年级.pdf",
		`AUX`:         "_AUX.pdf",
		`report.pdf`:  "report.pdf",
		`  数学   上册  `: "数学 上册.pdf",
	}
	for input, want := range tests {
		if got := SafeFilename(input); got != want {
			t.Errorf("SafeFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSafeFilenameUsesFallbackForEmptyInput(t *testing.T) {
	if got := SafeFilename("   "); got != "教材.pdf" {
		t.Fatalf("SafeFilename returned %q", got)
	}
}

func TestSafeFilenameCapsUTF8BasenameWithoutSplittingRunes(t *testing.T) {
	got := SafeFilename(strings.Repeat("教材", 100))
	base := strings.TrimSuffix(got, ".pdf")
	if !utf8.ValidString(got) {
		t.Fatalf("filename contains invalid UTF-8: %q", got)
	}
	if len([]byte(base)) > 180 {
		t.Fatalf("basename length = %d bytes, want at most 180", len([]byte(base)))
	}
}
