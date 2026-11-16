//go:build unix

package cluster

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// fifoTimeout bounds the test; reading a FIFO without a writer would otherwise block for good.
const fifoTimeout = 5 * time.Second

func TestFIFOIsRefusedWithoutBlocking(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := Contexts(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrKubeconfigNotFound) {
			t.Errorf("err = %v, want ErrKubeconfigNotFound", err)
		}
	case <-time.After(fifoTimeout):
		t.Fatal("Contexts blocked on a FIFO")
	}
}
