package agent

import (
	"os/exec"
	"syscall"
)

// AttachKillable 让 CommandContext 取消时杀掉整个进程组（含 Claude/Codex 子进程）。
func AttachKillable(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// 负 pid：向该进程组广播 SIGKILL
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
