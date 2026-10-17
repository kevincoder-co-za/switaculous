//go:build !windows

package switaculous

import "os/exec"

func hideConsoleWindow(command *exec.Cmd) {}
