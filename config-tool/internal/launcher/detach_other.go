//go:build !unix

package launcher

import "os/exec"

// detach is a no-op: the openers outside Unix hand off to the browser and exit.
func detach(*exec.Cmd) {}
