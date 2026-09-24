package lifecycle

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSecondInstanceSignalsFirstAndExits(t *testing.T) {
	id := fmt.Sprintf("smartedu-test-%d", time.Now().UnixNano())
	activated := make(chan struct{}, 1)
	release, err := AcquireSingleInstance(id, func() { activated <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	secondRelease, err := AcquireSingleInstance(id, nil)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second acquire error = %v, want ErrAlreadyRunning", err)
	}
	secondRelease()
	select {
	case <-activated:
	case <-time.After(2 * time.Second):
		t.Fatal("first instance did not receive activation")
	}
}

func TestReleasedInstanceCanBeAcquiredAgain(t *testing.T) {
	id := fmt.Sprintf("smartedu-test-release-%d", time.Now().UnixNano())
	release, err := AcquireSingleInstance(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	release()
	releaseAgain, err := AcquireSingleInstance(id, nil)
	if err != nil {
		t.Fatalf("acquire after release returned error: %v", err)
	}
	releaseAgain()
}

func TestReleaseDoesNotReportSecondLaunch(t *testing.T) {
	id := fmt.Sprintf("smartedu-test-stop-%d", time.Now().UnixNano())
	activated := make(chan struct{}, 1)
	release, err := AcquireSingleInstance(id, func() { activated <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	release()
	select {
	case <-activated:
		t.Fatal("release was reported as a second launch")
	case <-time.After(100 * time.Millisecond):
	}
}
