package cluster

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"k8s.io/client-go/tools/clientcmd"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestContexts(t *testing.T) {
	twoContexts := fixture(t, "two-contexts.yaml")
	other := fixture(t, "other.yaml")
	noCurrent := fixture(t, "no-current.yaml")
	empty := fixture(t, "empty.yaml")
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	oversized := filepath.Join(t.TempDir(), "oversized.yaml")
	if err := os.WriteFile(oversized, make([]byte, maxKubeconfigBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("current-context: at-home\ncontexts:\n- name: at-home\n  context: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		// kubeconfigEnv is the KUBECONFIG list; it is only read when explicit is empty.
		kubeconfigEnv []string
		explicit      string
		want          Kubeconfig
		wantNotFound  bool
		wantTooLarge  bool
		wantErr       bool
	}{
		{
			name:     "explicit file with two contexts",
			explicit: twoContexts,
			want:     Kubeconfig{Paths: []string{twoContexts}, Contexts: []string{"prod-eu-central", "staging"}, Current: "prod-eu-central"},
		},
		{
			name:     "explicit empty file",
			explicit: empty,
			want:     Kubeconfig{Paths: []string{empty}, Contexts: []string{}},
		},
		{
			name:     "explicit path under ~",
			explicit: "~/config.yaml",
			want:     Kubeconfig{Paths: []string{filepath.Join(home, "config.yaml")}, Contexts: []string{"at-home"}, Current: "at-home"},
		},
		{
			name:     "explicit path under ~ with the OS separator",
			explicit: "~" + string(filepath.Separator) + "config.yaml",
			want:     Kubeconfig{Paths: []string{filepath.Join(home, "config.yaml")}, Contexts: []string{"at-home"}, Current: "at-home"},
		},
		{
			name:     "relative explicit path resolves against the working directory",
			explicit: filepath.Join("testdata", "two-contexts.yaml"),
			want:     Kubeconfig{Paths: []string{twoContexts}, Contexts: []string{"prod-eu-central", "staging"}, Current: "prod-eu-central"},
		},
		{name: "explicit missing file", explicit: missing, wantNotFound: true},
		{name: "explicit directory", explicit: t.TempDir(), wantNotFound: true},
		{name: "explicit device", explicit: os.DevNull, wantNotFound: true},
		{name: "explicit file above the size limit", explicit: oversized, wantTooLarge: true},
		{name: "explicit malformed file", explicit: fixture(t, "malformed.yaml"), wantErr: true},
		{
			name:          "KUBECONFIG with two files merges them",
			kubeconfigEnv: []string{other, twoContexts},
			want:          Kubeconfig{Paths: []string{other, twoContexts}, Contexts: []string{"k3d-local", "prod-eu-central", "staging"}, Current: "k3d-local"},
		},
		{
			name:          "current context comes from the first file that sets it",
			kubeconfigEnv: []string{noCurrent, twoContexts},
			want:          Kubeconfig{Paths: []string{noCurrent, twoContexts}, Contexts: []string{"kind-kind", "prod-eu-central", "staging"}, Current: "prod-eu-central"},
		},
		{
			name:          "missing files in KUBECONFIG are skipped",
			kubeconfigEnv: []string{missing, twoContexts},
			want:          Kubeconfig{Paths: []string{twoContexts}, Contexts: []string{"prod-eu-central", "staging"}, Current: "prod-eu-central"},
		},
		{name: "device in KUBECONFIG is refused", kubeconfigEnv: []string{twoContexts, os.DevNull}, wantNotFound: true},
		{name: "directory in KUBECONFIG is refused", kubeconfigEnv: []string{t.TempDir()}, wantNotFound: true},
		{name: "file in KUBECONFIG above the size limit", kubeconfigEnv: []string{oversized}, wantTooLarge: true},
		{name: "path under a file in KUBECONFIG is refused", kubeconfigEnv: []string{filepath.Join(twoContexts, "config")}, wantNotFound: true},
		{
			name:          "KUBECONFIG with only missing files is empty",
			kubeconfigEnv: []string{missing},
			want:          Kubeconfig{Paths: []string{}, Contexts: []string{}},
		},
		{
			name:          "explicit path ignores KUBECONFIG",
			kubeconfigEnv: []string{other},
			explicit:      twoContexts,
			want:          Kubeconfig{Paths: []string{twoContexts}, Contexts: []string{"prod-eu-central", "staging"}, Current: "prod-eu-central"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv(clientcmd.RecommendedConfigPathEnvVar, strings.Join(tc.kubeconfigEnv, string(os.PathListSeparator)))
			got, err := Contexts(tc.explicit)
			switch {
			case tc.wantNotFound:
				if !errors.Is(err, ErrKubeconfigNotFound) {
					t.Fatalf("err = %v, want ErrKubeconfigNotFound", err)
				}
				return
			case tc.wantTooLarge:
				if !errors.Is(err, ErrKubeconfigTooLarge) {
					t.Fatalf("err = %v, want ErrKubeconfigTooLarge", err)
				}
				return
			case tc.wantErr:
				if err == nil || errors.Is(err, ErrKubeconfigNotFound) {
					t.Fatalf("err = %v, want a read error other than ErrKubeconfigNotFound", err)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Contexts = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMissingExplicitPathWrapsTheOSError(t *testing.T) {
	_, err := Contexts(filepath.Join(t.TempDir(), "missing.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want it to wrap fs.ErrNotExist", err)
	}
}

// The default ~/.kube/config is fixed when client-go initializes, so its absence is tested one level down.
func TestMissingDefaultFileIsEmpty(t *testing.T) {
	got, err := fromPrecedence([]string{filepath.Join(t.TempDir(), ".kube", "config")})
	if err != nil {
		t.Fatal(err)
	}
	if want := (Kubeconfig{Paths: []string{}, Contexts: []string{}}); !reflect.DeepEqual(got, want) {
		t.Errorf("fromPrecedence = %+v, want %+v", got, want)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	sep := string(filepath.Separator)
	tests := []struct{ path, want string }{
		{"~", home},
		{"~/", home},
		{"~/.kube/config", filepath.Join(home, ".kube", "config")},
		{"~" + sep + "config", filepath.Join(home, "config")},
		{"~nobody/config", "~nobody/config"},
		{"/etc/~/config", "/etc/~/config"},
		{"config", "config"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, err := expandHome(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("expandHome(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
