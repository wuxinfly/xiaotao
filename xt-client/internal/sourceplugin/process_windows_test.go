package sourceplugin

import (
	"os/exec"
	"testing"
)

func TestHideCommandWindow(t *testing.T) {
	cmd := exec.Command("example.exe")
	hideCommandWindow(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
		t.Fatal("plugin must start without a console window")
	}
}
