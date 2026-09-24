package auth

import (
	"context"
	"errors"
	"strings"
	"sync"

	"smartedu-downloader/internal/resource"
)

const maxTokenBytes = 16 * 1024

type tokenValidator interface {
	Validate(context.Context, string, string) error
}

type Status struct {
	Authenticated bool `json:"authenticated"`
}

type Session struct {
	mutex     sync.RWMutex
	validator tokenValidator
	token     string
}

func NewSession(validator tokenValidator) *Session {
	return &Session{validator: validator}
}

func (session *Session) SetValidated(ctx context.Context, token string, source resource.PDFSource) error {
	session.Clear()
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTokenBytes {
		return ErrInvalidToken
	}
	if session.validator == nil {
		return errors.New("session validator is not configured")
	}
	if len(source.Mirrors) == 0 {
		return errors.New("validation source has no mirrors")
	}
	if err := session.validator.Validate(ctx, token, source.Mirrors[0]); err != nil {
		return err
	}

	session.mutex.Lock()
	session.token = token
	session.mutex.Unlock()
	return nil
}

func (session *Session) Token() (string, bool) {
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	return session.token, session.token != ""
}

func (session *Session) Clear() {
	session.mutex.Lock()
	session.token = ""
	session.mutex.Unlock()
}

func (session *Session) Status() Status {
	session.mutex.RLock()
	defer session.mutex.RUnlock()
	return Status{Authenticated: session.token != ""}
}
