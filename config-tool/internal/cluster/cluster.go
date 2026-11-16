// Package cluster reads kubeconfig files to offer kube contexts. It never connects to a cluster
// and never writes a file.
package cluster

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// home is the "~" a user may type for the home directory, alone or followed by a separator.
const home = "~"

// maxKubeconfigBytes caps an explicit kubeconfig read; real files with embedded certificates stay
// far below it, while a mistyped path to a huge file must not be loaded whole.
const maxKubeconfigBytes = 8 << 20

var (
	// ErrKubeconfigNotFound reports an explicit kubeconfig path that is missing, unreadable or not
	// a regular file.
	ErrKubeconfigNotFound = errors.New("kubeconfig not found")
	// ErrKubeconfigTooLarge reports an explicit kubeconfig above maxKubeconfigBytes.
	ErrKubeconfigTooLarge = errors.New("kubeconfig too large")
)

// Kubeconfig is what the Target stage needs from the kubeconfig files: names only, no credentials
// and no server URLs.
type Kubeconfig struct {
	// Paths are the files actually read, in precedence order.
	Paths    []string `json:"paths"`
	Contexts []string `json:"contexts"`
	Current  string   `json:"current"`
}

// Contexts lists the contexts of explicitPath, or under clientcmd's default loading rules
// (KUBECONFIG, else ~/.kube/config) when it is empty. Missing default files yield an empty list;
// a missing or unreadable explicitPath yields an error wrapping ErrKubeconfigNotFound.
func Contexts(explicitPath string) (Kubeconfig, error) {
	if explicitPath == "" {
		return fromPrecedence(clientcmd.NewDefaultClientConfigLoadingRules().GetLoadingPrecedence())
	}
	path, err := expandHome(explicitPath)
	if err != nil {
		return Kubeconfig{}, err
	}
	data, err := readRegular(path)
	if err != nil {
		return Kubeconfig{}, err
	}
	config, err := clientcmd.Load(data)
	if err != nil {
		return Kubeconfig{}, fmt.Errorf("read kubeconfig %s: %w", path, err)
	}
	return summarize([]string{path}, config), nil
}

// fromPrecedence merges the existing files of precedence with clientcmd's rules: the first file
// that sets a context or current-context wins.
func fromPrecedence(precedence []string) (Kubeconfig, error) {
	var existing []string
	for _, path := range precedence {
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, path)
		}
	}
	if len(existing) == 0 {
		return summarize(nil, clientcmdapi.NewConfig()), nil
	}
	// Without MigrationRules, Load never copies a legacy kubeconfig into place.
	rules := &clientcmd.ClientConfigLoadingRules{Precedence: existing}
	config, err := rules.Load()
	if err != nil {
		return Kubeconfig{}, fmt.Errorf("read kubeconfig: %w", err)
	}
	return summarize(existing, config), nil
}

// readRegular reads path, which must be a regular file of at most maxKubeconfigBytes. The check
// comes before the open, so a FIFO or device such as /dev/zero can neither block nor fill memory.
func readRegular(path string) ([]byte, error) {
	if err := checkRegular(os.Stat(path)); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
	}
	defer f.Close()
	// The path may have been swapped between Stat and Open.
	if err := checkRegular(f.Stat()); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxKubeconfigBytes+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
	case len(data) > maxKubeconfigBytes:
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrKubeconfigTooLarge, path, maxKubeconfigBytes)
	}
	return data, nil
}

func checkRegular(info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
	case !info.Mode().IsRegular():
		return fmt.Errorf("%w: %s is not a regular file", ErrKubeconfigNotFound, info.Name())
	}
	return nil
}

func summarize(paths []string, config *clientcmdapi.Config) Kubeconfig {
	contexts := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		contexts = append(contexts, name)
	}
	slices.Sort(contexts)
	if paths == nil {
		paths = []string{}
	}
	return Kubeconfig{Paths: paths, Contexts: contexts, Current: config.CurrentContext}
}

// expandHome replaces a leading "~", "~/" or (on Windows) "~\" with the home directory.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, home)
	if !ok || (rest != "" && rest[0] != '/' && rest[0] != filepath.Separator) {
		return path, nil
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %s: %w", path, err)
	}
	return filepath.Join(dir, rest), nil
}
