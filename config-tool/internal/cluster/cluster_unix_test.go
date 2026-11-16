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
	tests := []struct {
		name          string
		kubeconfigEnv string
		explicit      string
	}{
		{name: "explicit path", explicit: fifo},
		{name: "KUBECONFIG", kubeconfigEnv: fifo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(clientcmd.RecommendedConfigPathEnvVar, tc.kubeconfigEnv)
			done := make(chan error, 1)
			go func() {
				_, err := Contexts(tc.explicit)
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
		})
	}
}
