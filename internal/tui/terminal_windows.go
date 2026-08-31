//go:build windows
// +build windows

package tui

import (
	"syscall"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	enableLineInput      = 0x0002
	enableEchoInput      = 0x0004
	enableProcessedInput = 0x0001
	enableVirtualTerminalInput = 0x0200
)

// RawMode enables raw input on Windows by disabling line buffering and echo,
// while preserving the ability to read virtual terminal (ANSI) sequences.
func RawMode() (func(), error) {
	handle, err := syscall.GetStdHandle(syscall.STD_INPUT_HANDLE)
	if err != nil {
		return func() {}, err
	}

	var mode uint32
	ret, _, errCode := getConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return func() {}, errCode
	}

	// Disable line input, echo, and processed input.
	// Enable virtual terminal input so we receive ANSI escape sequences (e.g. arrow keys).
	newMode := mode &^ (enableLineInput | enableEchoInput | enableProcessedInput)
	newMode |= enableVirtualTerminalInput

	ret, _, errCode = setConsoleMode.Call(uintptr(handle), uintptr(newMode))
	if ret == 0 {
		return func() {}, errCode
	}

	// Return a cleanup function to restore the original terminal mode.
	cleanup := func() {
		setConsoleMode.Call(uintptr(handle), uintptr(mode))
	}

	return cleanup, nil
}
