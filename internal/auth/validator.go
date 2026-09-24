package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var (
	ErrInvalidToken       = errors.New("access token is invalid or expired")
	ErrInvalidPDFResponse = errors.New("protected resource did not return a PDF")
)

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Validator struct {
	client HTTPClient
}

func NewValidator(client HTTPClient) *Validator {
	return &Validator{client: client}
}

func (validator *Validator) Validate(ctx context.Context, token, mirror string) error {
	if validator == nil || validator.client == nil {
		return errors.New("token validator is not configured")
	}
	parsed, err := url.Parse(mirror)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("protected resource URL is invalid")
	}
	query := parsed.Query()
	query.Set("accessToken", token)
	parsed.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return fmt.Errorf("create token validation request: %w", err)
	}
	request.Header.Set("Range", "bytes=0-4")
	request.Header.Set("Accept", "application/pdf")
	response, err := validator.client.Do(request)
	if err != nil {
		return fmt.Errorf("validate access token: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return ErrInvalidToken
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("token validation returned HTTP %d", response.StatusCode)
	}
	prefix, err := io.ReadAll(io.LimitReader(response.Body, 5))
	if err != nil {
		return fmt.Errorf("read token validation response: %w", err)
	}
	if !strings.HasPrefix(string(prefix), "%PDF") {
		return ErrInvalidPDFResponse
	}
	return nil
}
