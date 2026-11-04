package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestVisibleWindowLaunchFromHiddenProcess(t *testing.T) {
	if mode := os.Getenv("XIAOTAO_LAUNCH_TEST"); mode != "" {
		if mode == "parent" {
			os.Setenv("XIAOTAO_LAUNCH_TEST", "child")
			if err := Start(os.Args[0], "-test.run=^TestVisibleWindowLaunchFromHiddenProcess$", "--", os.Getenv("XIAOTAO_LAUNCH_FLAG")); err != nil {
				os.Exit(2)
			}
			return
		}
		var info windows.StartupInfo
		windows.GetStartupInfo(&info)
		expected := uint16(windows.SW_SHOWNORMAL)
		if os.Getenv("XIAOTAO_LAUNCH_FLAG") == "--hud-window" {
			expected = windows.SW_SHOWNOACTIVATE
		}
		if info.Flags&windows.STARTF_USESHOWWINDOW != 0 && info.ShowWindow == expected {
			os.WriteFile(os.Getenv("XIAOTAO_LAUNCH_RESULT"), []byte("visible"), 0600)
		}
		os.Exit(0)
	}
	for _, flag := range []string{"--window", "--hud-window"} {
		result := filepath.Join(t.TempDir(), "result")
		command := exec.Command(os.Args[0], "-test.run=^TestVisibleWindowLaunchFromHiddenProcess$")
		command.Env = append(os.Environ(), "XIAOTAO_LAUNCH_TEST=parent", "XIAOTAO_LAUNCH_RESULT="+result, "XIAOTAO_LAUNCH_FLAG="+flag)
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := command.Run(); err != nil {
			t.Fatal(err)
		}
		passed := false
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if data, err := os.ReadFile(result); err == nil && string(data) == "visible" {
				passed = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if !passed {
			t.Fatalf("%s child did not receive expected startup state", flag)
		}
	}
}
