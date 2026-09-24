package main

import (
	"context"
	"errors"
	"testing"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestFitWindowSizeLeavesRoomForTitleBarAndTaskbarOnScaledDisplay(t *testing.T) {
	width, height := fitWindowSize(1092, 614)

	if width != 996 || height != 518 {
		t.Fatalf("fitWindowSize(1092, 614) = %dx%d, want 996x518", width, height)
	}
}

func TestWindowSizeUsesValidCurrentScreenWhenScreenDiscoveryIsPartial(t *testing.T) {
	current := wailsruntime.Screen{IsCurrent: true}
	current.Size.Width = 1092
	current.Size.Height = 614
	screens := []wailsruntime.Screen{current}

	width, height := windowSizeForScreens(screens, errors.New("secondary monitor DPI unavailable"))
	if width != 996 || height != 518 {
		t.Fatalf("windowSizeForScreens() = %dx%d, want 996x518", width, height)
	}
}

func TestWindowActivationWaitsUntilInitialFitCompletes(t *testing.T) {
	activationCount := 0
	window := &windowRuntime{activateWindow: func(context.Context) { activationCount++ }}
	window.SetContext(context.Background())

	window.Activate()
	if activationCount != 0 {
		t.Fatalf("activation ran %d times before initial fit, want 0", activationCount)
	}

	window.MarkReady()
	if activationCount != 1 {
		t.Fatalf("activation ran %d times after initial fit, want 1", activationCount)
	}
}
