package agent

import (
	"os/exec"
	"testing"
)

func TestAttachKillableSetsCancel(t *testing.T) {
	cmd := exec.Command("true")
	AttachKillable(cmd)
	if cmd.Cancel == nil {
		t.Fatal("Cancel must be set so context cancel kills the process group")
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("Setpgid required so children receive SIGKILL")
	}
}
