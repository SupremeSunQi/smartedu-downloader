//go:build !windows

package lifecycle

import (
	"errors"
	"sync"
)

var ErrAlreadyRunning = errors.New("application is already running")

var testInstances sync.Map

func AcquireSingleInstance(id string, onSecondLaunch func()) (func(), error) {
	if existing, loaded := testInstances.LoadOrStore(id, onSecondLaunch); loaded {
		if callback, ok := existing.(func()); ok && callback != nil {
			callback()
		}
		return func() {}, ErrAlreadyRunning
	}
	var once sync.Once
	return func() { once.Do(func() { testInstances.Delete(id) }) }, nil
}
