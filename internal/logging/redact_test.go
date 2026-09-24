package logging

import (
	"strings"
	"testing"
)

func TestRedactRemovesSecretsFromURLsHeadersAndJSON(t *testing.T) {
	input := `GET /book.pdf?accessToken=abc&v=1 Authorization: MAC id="abc" {"password":"secret","token":"abc","mac_key":"key"}`
	output := Redact(input)
	for _, secret := range []string{"abc", "secret", "key"} {
		if strings.Contains(output, secret) {
			t.Errorf("Redact retained %q: %s", secret, output)
		}
	}
	if !strings.Contains(output, "[REDACTED]") || !strings.Contains(output, "v=1") {
		t.Fatalf("Redact removed safe context or omitted marker: %s", output)
	}
}

func TestRedactIsCaseInsensitive(t *testing.T) {
	output := Redact(`?ACCESS_TOKEN=topsecret {"Password":"hidden"}`)
	if strings.Contains(output, "topsecret") || strings.Contains(output, "hidden") {
		t.Fatalf("Redact retained mixed-case secrets: %s", output)
	}
}
