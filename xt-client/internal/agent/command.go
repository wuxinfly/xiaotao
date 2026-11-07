package agent

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
)

// windowsUTF8Preamble makes PowerShell write UTF-8 to its redirected output and
// to the stdin pipe of native children. A Chinese Windows console defaults to code
// page 936, so cmdlet output arrived as GBK and every non-ASCII byte on a stdin
// pipe was silently replaced with "?", which corrupted piped JSON such as the
// stage-save payload before save_progress.py saw it. File reading is left alone so
// existing GBK files still read correctly; commands that pipe a file must pass
// -Encoding UTF8 explicitly.
const windowsUTF8Preamble = `try{[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)}catch{};` +
	`$OutputEncoding=[Text.UTF8Encoding]::new($false);`

// shellCommand builds the platform shell invocation; a nil context is allowed for
// callers that only need a plain exec.Cmd.
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS != "windows" {
		if ctx == nil {
			return exec.Command("/bin/sh", "-lc", command)
		}
		return exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}
	args := []string{"-NoProfile", "-NonInteractive", "-Command", windowsUTF8Preamble + command}
	if ctx == nil {
		return exec.Command("powershell.exe", args...)
	}
	return exec.CommandContext(ctx, "powershell.exe", args...)
}

// childUTF8Env keeps python children on UTF-8 stdio and replaces inherited
// duplicates so the last value wins on Windows.
func childUTF8Env(base []string) []string {
	extra := []string{"PYTHONIOENCODING=utf-8"}
	env := make([]string, 0, len(base)+len(extra))
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		keep := true
		for _, added := range extra {
			if addedKey, _, _ := strings.Cut(added, "="); strings.EqualFold(key, addedKey) {
				keep = false
				break
			}
		}
		if keep {
			env = append(env, item)
		}
	}
	return append(env, extra...)
}
