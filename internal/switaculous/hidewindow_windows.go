package switaculous

import (
	"os/exec"
	"syscall"
)

// Helper processes such as the PowerShell that hosts the Explorer file
// dialog must not pop up a console window of their own.
func hideConsoleWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
