//go:build windows

package main

import (
	"os/exec"
	"strconv"
)

func setpgid(cmd *exec.Cmd) {
	// Not applicable on Windows
}

func killProcess(cmd *exec.Cmd) error {
	return exec.Command("taskkill", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
