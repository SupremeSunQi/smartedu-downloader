package browserauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

func TestCandidatesIgnoresUnrelatedOriginsAndKeys(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	chromeRoot := t.TempDir()
	writeProfile(t, chromeRoot, "Default", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey):          sdkRecord(t, "valid-token", now.Add(time.Hour)),
		chromiumRecordKey("https://other.example", smartEduTokenKey): sdkRecord(t, "wrong-origin", now.Add(time.Hour)),
		chromiumRecordKey(smartEduOrigin, "other-key"):               sdkRecord(t, "wrong-key", now.Add(time.Hour)),
	})

	reader := NewReader(Options{ChromeRoot: chromeRoot, Now: func() time.Time { return now }})
	candidates, err := reader.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{{Browser: "Chrome", Profile: "Default", Token: "valid-token"}}
	if len(candidates) != len(want) || candidates[0] != want[0] {
		t.Fatalf("Candidates() = %#v, want %#v", candidates, want)
	}
}

func TestCandidatesReadsTokensAcrossSmartEduSubdomains(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	chromeRoot := t.TempDir()
	otherSDKKey := "ND_UC_AUTH-another-app&ncet-xedu&token"
	writeProfile(t, chromeRoot, "Default", map[string][]byte{
		chromiumRecordKey("https://basic.smartedu.cn", smartEduTokenKey):       sdkRecord(t, "basic-token", now.Add(time.Hour)),
		chromiumRecordKey("https://www.smartedu.cn", otherSDKKey):              sdkRecord(t, "www-token", now.Add(time.Hour)),
		chromiumRecordKey("https://auth.smartedu.cn", smartEduTokenKey):        sdkRecord(t, "auth-token", now.Add(time.Hour)),
		chromiumRecordKey("https://smartedu.cn", smartEduTokenKey):             sdkRecord(t, "root-token", now.Add(time.Hour)),
		chromiumRecordKey("https://not-smartedu.cn", smartEduTokenKey):         sdkRecord(t, "lookalike-token", now.Add(time.Hour)),
		chromiumRecordKey("https://smartedu.cn.attacker.example", otherSDKKey): sdkRecord(t, "suffix-token", now.Add(time.Hour)),
		chromiumRecordKey("http://www.smartedu.cn", smartEduTokenKey):          sdkRecord(t, "http-token", now.Add(time.Hour)),
		chromiumRecordKey("https://www.smartedu.cn", "unrelated-key"):          sdkRecord(t, "unrelated-token", now.Add(time.Hour)),
	})

	reader := NewReader(Options{ChromeRoot: chromeRoot, Now: func() time.Time { return now }})
	candidates, err := reader.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"auth-token", "basic-token", "root-token", "www-token"}
	if len(candidates) != len(want) {
		t.Fatalf("Candidates() returned %d candidates, want %d", len(candidates), len(want))
	}
	for index, token := range want {
		if candidates[index].Token != token {
			t.Fatalf("Candidates()[%d].Token = %q, want %q", index, candidates[index].Token, token)
		}
	}
}

func TestCandidatesSkipsExpiredSubdomainTokenWhenAnotherIsValid(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	chromeRoot := t.TempDir()
	writeProfile(t, chromeRoot, "Default", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey):            sdkRecord(t, "expired-token", now.Add(-time.Minute)),
		chromiumRecordKey("https://www.smartedu.cn", smartEduTokenKey): sdkRecord(t, "valid-token", now.Add(time.Hour)),
	})

	reader := NewReader(Options{ChromeRoot: chromeRoot, Now: func() time.Time { return now }})
	candidates, err := reader.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Token != "valid-token" {
		t.Fatalf("Candidates() = %#v, want one valid www.smartedu.cn token", candidates)
	}
}

func TestCandidatesReadsDeterministicChromeAndEdgeProfiles(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	chromeRoot := t.TempDir()
	edgeRoot := t.TempDir()
	writeProfile(t, chromeRoot, "Profile 2", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey): sdkRecord(t, "chrome-profile", now.Add(time.Hour)),
	})
	writeProfile(t, chromeRoot, "Default", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey): sdkRecord(t, "chrome-default", now.Add(time.Hour)),
	})
	writeProfile(t, edgeRoot, "Default", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey): sdkRecord(t, "edge-default", now.Add(time.Hour)),
	})

	reader := NewReader(Options{ChromeRoot: chromeRoot, EdgeRoot: edgeRoot, Now: func() time.Time { return now }})
	candidates, err := reader.Candidates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{
		{Browser: "Chrome", Profile: "Default", Token: "chrome-default"},
		{Browser: "Chrome", Profile: "Profile 2", Token: "chrome-profile"},
		{Browser: "Edge", Profile: "Default", Token: "edge-default"},
	}
	if len(candidates) != len(want) {
		t.Fatalf("Candidates() returned %d candidates, want %d: %#v", len(candidates), len(want), candidates)
	}
	for index := range want {
		if candidates[index] != want[index] {
			t.Fatalf("Candidates()[%d] = %#v, want %#v", index, candidates[index], want[index])
		}
	}
}

func TestDecodeSDKValueRejectsExpiredMalformedAndOversizedValues(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value []byte
		want  error
	}{
		{name: "expired", value: sdkRecord(t, "expired-token", now.Add(-time.Hour)), want: ErrExpired},
		{name: "malformed", value: []byte("\x01not-json"), want: ErrIncompatibleStorage},
		{name: "oversized", value: bytes.Repeat([]byte{'x'}, maxStoredValueBytes+1), want: ErrIncompatibleStorage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeSDKValue(test.value, now)
			if !errors.Is(err, test.want) {
				t.Fatalf("decodeSDKValue() error = %v, want %v", err, test.want)
			}
			if err != nil && bytes.Contains([]byte(err.Error()), []byte("expired-token")) {
				t.Fatal("decode error exposed token material")
			}
		})
	}
}

func TestCandidatesReportsExpiredWhenOnlyKnownSessionExpired(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	chromeRoot := t.TempDir()
	writeProfile(t, chromeRoot, "Default", map[string][]byte{
		chromiumRecordKey(smartEduOrigin, smartEduTokenKey): sdkRecord(t, "expired-token", now.Add(-time.Minute)),
	})

	reader := NewReader(Options{ChromeRoot: chromeRoot, Now: func() time.Time { return now }})
	_, err := reader.Candidates(context.Background())
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Candidates() error = %v, want ErrExpired", err)
	}
	if err != nil && bytes.Contains([]byte(err.Error()), []byte("expired-token")) {
		t.Fatal("Candidates() error exposed token material")
	}
}

func TestCandidatesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader := NewReader(Options{ChromeRoot: t.TempDir()})
	_, err := reader.Candidates(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Candidates() error = %v, want context.Canceled", err)
	}
}

func writeProfile(t *testing.T, browserRoot, profile string, records map[string][]byte) {
	t.Helper()
	path := filepath.Join(browserRoot, profile, "Local Storage", "leveldb")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	database, err := leveldb.OpenFile(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range records {
		if err := database.Put([]byte(key), value, nil); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
}

func chromiumRecordKey(origin, key string) string {
	return "_" + origin + "\x00\x01" + key
}

func sdkRecord(t *testing.T, token string, expires time.Time) []byte {
	t.Helper()
	tokenJSON, err := json.Marshal(map[string]string{"access_token": token})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := json.Marshal(map[string]any{"value": string(tokenJSON), "expire": expires.UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte{1}, wrapper...)
}
