//go:build unix

package launcher

import (
	"os/exec"
	"syscall"
)

// detach starts cmd in its own session, so Ctrl+C on dsc-config does not reach a browser
// that xdg-open launched directly.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
