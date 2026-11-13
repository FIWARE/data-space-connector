// Package launcher opens the dsc-config UI for the user.
package launcher

import (
	"fmt"
	"os/exec"
	"runtime"
)

// LaunchSource issues a single-use launch URL; the run token must never appear in it.
type LaunchSource interface {
	LaunchURL() (string, error)
}

// Open opens a fresh launch URL from src in the default browser without waiting for it.
// The URL lands in the browser's argv, readable by every local user, hence the launch code.
func Open(src LaunchSource) error {
	url, err := src.LaunchURL()
	if err != nil {
		return err
	}
	name, args, err := command(runtime.GOOS, url)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", name, err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// command builds the opener's argv for goos.
func command(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "linux", "freebsd", "netbsd", "openbsd", "dragonfly", "illumos", "solaris":
		return "xdg-open", []string{url}, nil
	default:
		return "", nil, fmt.Errorf("opening a browser is not supported on %s", goos)
	}
}
