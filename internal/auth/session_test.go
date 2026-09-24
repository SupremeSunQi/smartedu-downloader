package auth

import (
	"context"
	"testing"

	"smartedu-downloader/internal/resource"
)

func TestSessionStoresOnlyTokenThatPassesProtectedRangeProbe(t *testing.T) {
	server := protectedPDFServer(t, "valid-token")
	session := NewSession(NewValidator(server.Client()))
	source := resource.PDFSource{Mirrors: []string{server.URL + "/book.pdf"}}

	if err := session.SetValidated(context.Background(), "valid-token", source); err != nil {
		t.Fatalf("SetValidated returned error: %v", err)
	}
	token, ok := session.Token()
	if !ok || token != "valid-token" {
		t.Fatalf("Token = %q, %v", token, ok)
	}
	if !session.Status().Authenticated {
		t.Fatal("session status is not authenticated")
	}
}

func TestSessionRejectsUnauthorizedTokenWithoutRetainingIt(t *testing.T) {
	server := protectedPDFServer(t, "valid-token")
	session := NewSession(NewValidator(server.Client()))
	source := resource.PDFSource{Mirrors: []string{server.URL + "/book.pdf"}}

	if err := session.SetValidated(context.Background(), "expired", source); err != ErrInvalidToken {
		t.Fatalf("SetValidated error = %v, want ErrInvalidToken", err)
	}
	if _, ok := session.Token(); ok {
		t.Fatal("invalid token was retained")
	}
}

func TestSessionClearRemovesValidatedToken(t *testing.T) {
	server := protectedPDFServer(t, "valid-token")
	session := NewSession(NewValidator(server.Client()))
	source := resource.PDFSource{Mirrors: []string{server.URL + "/book.pdf"}}
	if err := session.SetValidated(context.Background(), "valid-token", source); err != nil {
		t.Fatal(err)
	}

	session.Clear()
	if _, ok := session.Token(); ok || session.Status().Authenticated {
		t.Fatal("Clear retained authentication state")
	}
}
