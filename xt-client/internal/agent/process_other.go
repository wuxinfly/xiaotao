//go:build !windows

package agent

import "os/exec"

func hideCommandWindow(cmd *exec.Cmd) {}
