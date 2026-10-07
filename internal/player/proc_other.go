//go:build !linux

package player

import "os/exec"

// No parent-death signal outside Linux; Stop on a normal exit is all we have.
func setProcAttr(*exec.Cmd) {}
