//go:build unix

package launcher

import (
	"os/exec"
	"testing"
)

func TestDetachStartsANewSession(t *testing.T) {
	cmd := exec.Command("true")
	detach(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Errorf("SysProcAttr = %+v, want Setsid", cmd.SysProcAttr)
	}
}
