//go:build !windows

package office

import "os/exec"

func hideCommandWindow(cmd *exec.Cmd) {}
