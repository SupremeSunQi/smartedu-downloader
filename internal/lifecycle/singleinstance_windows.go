//go:build windows

package lifecycle

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

var ErrAlreadyRunning = errors.New("application is already running")

func AcquireSingleInstance(id string, onSecondLaunch func()) (func(), error) {
	name := sanitizeObjectName(id)
	mutexName, err := windows.UTF16PtrFromString(`Local\` + name + `-mutex`)
	if err != nil {
		return func() {}, err
	}
	mutex, err := windows.CreateMutex(nil, false, mutexName)
	alreadyRunning := errors.Is(err, windows.ERROR_ALREADY_EXISTS)
	if err != nil && !alreadyRunning {
		return func() {}, fmt.Errorf("create instance mutex: %w", err)
	}
	eventName, err := windows.UTF16PtrFromString(`Local\` + name + `-activate`)
	if err != nil {
		windows.CloseHandle(mutex)
		return func() {}, err
	}
	event, err := windows.CreateEvent(nil, 0, 0, eventName)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(mutex)
		return func() {}, fmt.Errorf("create activation event: %w", err)
	}
	if alreadyRunning {
		_ = windows.SetEvent(event)
		windows.CloseHandle(event)
		windows.CloseHandle(mutex)
		return func() {}, ErrAlreadyRunning
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		for {
			result, _ := windows.WaitForSingleObject(event, 250)
			select {
			case <-stop:
				return
			default:
			}
			if result == windows.WAIT_OBJECT_0 && onSecondLaunch != nil {
				onSecondLaunch()
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(stop)
			_ = windows.SetEvent(event)
			<-done
			windows.CloseHandle(event)
			windows.CloseHandle(mutex)
		})
	}, nil
}

func sanitizeObjectName(id string) string {
	var builder strings.Builder
	for _, char := range id {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "SmartEduDownloader"
	}
	return builder.String()
}
