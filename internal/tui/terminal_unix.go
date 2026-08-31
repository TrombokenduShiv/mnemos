//go:build !windows
// +build !windows

package tui

import (
	"os"
	"os/exec"
)

// RawMode enables raw input on Unix systems using the standard stty utility.
// We execute "stty -icanon -echo" to disable line buffering and echo.
func RawMode() (func(), error) {
	// First, try to get the original stty state
	origState, err := exec.Command("stty", "-g").Output()
	if err != nil {
		return func() {}, err
	}

	// Disable canonical mode (line buffering) and echo
	cmd := exec.Command("stty", "-icanon", "-echo")
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return func() {}, err
	}

	// Return a cleanup function to restore the terminal state
	cleanup := func() {
		restoreCmd := exec.Command("stty", string(origState))
		restoreCmd.Stdin = os.Stdin
		restoreCmd.Run()
	}

	return cleanup, nil
}
