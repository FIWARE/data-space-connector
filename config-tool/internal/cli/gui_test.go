package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FIWARE/data-space-connector/config-tool/internal/launcher"
)

// guiTimeout bounds a runGUI call that should already have returned.
const guiTimeout = 10 * time.Second

var (
	errFakeWindow = errors.New("fake window failed")
	errFakeTab    = errors.New("fake tab failed")
)

func TestRunGUILaunchPaths(t *testing.T) {
	tests := []struct {
		name string
		// window and tab get the run's parent cancel, standing in for SIGINT.
		window     func(ctx context.Context, stop context.CancelFunc) error
		tab        func(stop context.CancelFunc) error
		wantTab    bool
		wantStderr string
	}{
		{
			name:   "closing the window ends the run without a tab",
			window: func(context.Context, context.CancelFunc) error { return nil },
		},
		{
			name: "a signal closes the open window",
			window: func(ctx context.Context, stop context.CancelFunc) error {
				stop()
				<-ctx.Done()
				return nil
			},
		},
		{
			name:    "a build without a window opens a tab silently",
			window:  func(context.Context, context.CancelFunc) error { return launcher.ErrNoWindow },
			tab:     func(stop context.CancelFunc) error { stop(); return nil },
			wantTab: true,
		},
		{
			name:       "a failed window is reported and falls back to a tab",
			window:     func(context.Context, context.CancelFunc) error { return errFakeWindow },
			tab:        func(stop context.CancelFunc) error { stop(); return nil },
			wantTab:    true,
			wantStderr: "could not open a window: " + errFakeWindow.Error(),
		},
		{
			name:       "a failed tab points at the printed URL",
			window:     func(context.Context, context.CancelFunc) error { return launcher.ErrNoWindow },
			tab:        func(stop context.CancelFunc) error { stop(); return errFakeTab },
			wantTab:    true,
			wantStderr: "open the URL above instead",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			tabOpened := false
			var launchURL string
			replaceLaunchers(t,
				func(ctx context.Context, _ launcher.LaunchSource) error { return tc.window(ctx, stop) },
				func(src launcher.LaunchSource) error {
					tabOpened = true
					launchURL, _ = src.LaunchURL()
					return tc.tab(stop)
				})
			var stdout, stderr bytes.Buffer

			err := runGUIWithin(t, ctx, GUIOptions{Stdout: &stdout, Stderr: &stderr})

			if err != nil {
				t.Fatalf("runGUI: %v", err)
			}
			if tabOpened != tc.wantTab {
				t.Errorf("tab opened = %v, want %v", tabOpened, tc.wantTab)
			}
			if tc.wantTab && !strings.Contains(launchURL, "launch=") {
				t.Errorf("tab got %q, want a launch URL", launchURL)
			}
			if !strings.Contains(stdout.String(), "token=") {
				t.Errorf("stdout = %q, want the token URL", stdout.String())
			}
			if got := stderr.String(); tc.wantStderr == "" && got != "" || !strings.Contains(got, tc.wantStderr) {
				t.Errorf("stderr = %q, want %q", got, tc.wantStderr)
			}
		})
	}
}

func replaceLaunchers(t *testing.T, window func(context.Context, launcher.LaunchSource) error, tab func(launcher.LaunchSource) error) {
	t.Helper()
	prevWindow, prevTab := openWindow, openTab
	openWindow, openTab = window, tab
	t.Cleanup(func() { openWindow, openTab = prevWindow, prevTab })
}

func runGUIWithin(t *testing.T, ctx context.Context, opts GUIOptions) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- runGUI(ctx, opts) }()
	select {
	case err := <-done:
		return err
	case <-time.After(guiTimeout):
		t.Fatal("runGUI did not return")
		return nil
	}
}
