// Package cluster reads kubeconfig files to offer kube contexts. It never connects to a cluster
// and never writes a file.
package cluster

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// home is the "~" a user may type for the home directory, alone or followed by a separator.
const home = "~"

// maxKubeconfigBytes caps every kubeconfig read; real files with embedded certificates stay
// far below it, while a mistyped path to a huge file must not be loaded whole.
const maxKubeconfigBytes = 8 << 20

var (
	// ErrKubeconfigNotFound reports a kubeconfig path that is missing, unreadable or not a regular
	// file; the wrapped error says which.
	ErrKubeconfigNotFound = errors.New("cannot read kubeconfig")
	// ErrKubeconfigTooLarge reports a kubeconfig above maxKubeconfigBytes.
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
// (KUBECONFIG, else ~/.kube/config) when it is empty. Missing default files are skipped; a missing
// explicitPath, or any file that is not a regular one, yields an error wrapping ErrKubeconfigNotFound.
// A relative explicitPath is resolved against the working directory, and Paths holds the result.
func Contexts(explicitPath string) (Kubeconfig, error) {
	if explicitPath == "" {
		return fromPrecedence(clientcmd.NewDefaultClientConfigLoadingRules().GetLoadingPrecedence())
	}
	path, err := expandHome(explicitPath)
	if err != nil {
		return Kubeconfig{}, err
	}
	// The browser cannot see the tool's working directory, so report the file actually read.
	if path, err = filepath.Abs(path); err != nil {
		return Kubeconfig{}, fmt.Errorf("resolve %s: %w", explicitPath, err)
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
// that sets a context or current-context wins. Only a file that does not exist is skipped; every
// other one is read through readRegular, so it gets the same checks as an explicit path.
func fromPrecedence(precedence []string) (Kubeconfig, error) {
	var existing []string
	merged := clientcmdapi.NewConfig()
	for _, path := range precedence {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		data, err := readRegular(path)
		if err != nil {
			return Kubeconfig{}, err
		}
		config, err := clientcmd.Load(data)
		if err != nil {
			return Kubeconfig{}, fmt.Errorf("read kubeconfig %s: %w", path, err)
		}
		for name, context := range config.Contexts {
			if _, ok := merged.Contexts[name]; !ok {
				merged.Contexts[name] = context
			}
		}
		if merged.CurrentContext == "" {
			merged.CurrentContext = config.CurrentContext
		}
		existing = append(existing, path)
	}
	return summarize(existing, merged), nil
}

// readRegular reads path, which must pass checkFile. The check comes before the open, so a FIFO
// or device such as /dev/zero can neither block nor fill memory.
func readRegular(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err := checkFile(path, info, err); err != nil {
		return nil, err
	}
	return readOpened(path)
}

// readOpened opens path without blocking and re-checks what it opened, since the path may have
// been swapped for a FIFO or device after readRegular's stat.
func readOpened(path string) ([]byte, error) {
	f, err := os.OpenFile(path, openFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err := checkFile(path, info, err); err != nil {
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

func checkFile(path string, info os.FileInfo, err error) error {
	switch {
	case err != nil:
		return fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
	case !info.Mode().IsRegular():
		return fmt.Errorf("%w: %s is not a regular file", ErrKubeconfigNotFound, path)
	case info.Size() > maxKubeconfigBytes:
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrKubeconfigTooLarge, path, maxKubeconfigBytes)
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
