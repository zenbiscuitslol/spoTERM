package player

import (
	"os/exec"
	"syscall"
)

// setProcAttr makes the kernel send SIGTERM to go-librespot if spotiCLI dies
// without cleaning up (crash, kill -9), so no daemon is left playing.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
