//go:build unix

package cluster

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"k8s.io/client-go/tools/clientcmd"
)

// fifoTimeout bounds the test; reading a FIFO without a writer would otherwise block for good.
const fifoTimeout = 5 * time.Second

func TestFIFOIsRefusedWithoutBlocking(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	contexts := func(explicit string) func() error {
		return func() error {
			_, err := Contexts(explicit)
			return err
		}
	}
	tests := []struct {
		name          string
		kubeconfigEnv string
		read          func() error
	}{
		{name: "explicit path", read: contexts(fifo)},
		{name: "KUBECONFIG", kubeconfigEnv: fifo, read: contexts("")},
		// readOpened is what runs when the path is swapped for a FIFO after the stat check.
		{name: "swapped after stat", read: func() error {
			_, err := readOpened(fifo)
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(clientcmd.RecommendedConfigPathEnvVar, tc.kubeconfigEnv)
			done := make(chan error, 1)
			go func() { done <- tc.read() }()
			select {
			case err := <-done:
				if !errors.Is(err, ErrKubeconfigNotFound) {
					t.Errorf("err = %v, want ErrKubeconfigNotFound", err)
				}
			case <-time.After(fifoTimeout):
				t.Fatal("Contexts blocked on a FIFO")
			}
		})
	}
}
