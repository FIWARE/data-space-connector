//go:build !wails

package launcher

import (
	"context"
	"errors"
	"testing"
)

type noLaunch struct{}

func (noLaunch) LaunchURL() (string, error) { return "", errors.New("must not be called") }

func TestWindowWithoutWailsTag(t *testing.T) {
	if err := Window(context.Background(), noLaunch{}); !errors.Is(err, ErrNoWindow) {
		t.Fatalf("Window() = %v, want ErrNoWindow", err)
	}
}
