// Package cluster reads kubeconfig files to offer kube contexts. It never connects to a cluster
// and never writes a file.
package cluster

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// homePrefix is the "~/" a user may type for the home directory.
const homePrefix = "~" + string(filepath.Separator)

// ErrKubeconfigNotFound reports an explicit kubeconfig path that is missing or unreadable.
var ErrKubeconfigNotFound = errors.New("kubeconfig not found")

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
	data, err := os.ReadFile(path)
	if err != nil {
		return Kubeconfig{}, fmt.Errorf("%w: %w", ErrKubeconfigNotFound, err)
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

func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, homePrefix) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand %s: %w", path, err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, homePrefix)), nil
}
