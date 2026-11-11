//go:build !windows

package sourceplugin

import "os/exec"

func hideCommandWindow(cmd *exec.Cmd) {}
