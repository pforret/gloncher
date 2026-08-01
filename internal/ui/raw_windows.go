//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	enableVirtualTerminalProcessing = 0x0004
	enableLineInput                 = 0x0002
	enableEchoInput                 = 0x0004
	enableProcessedInput            = 0x0001
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// MakeRaw enables ANSI escape processing on the output handle and turns off
// line buffering and echo on the input handle, so the renderer's escape
// sequences work and single keypresses arrive immediately.
func MakeRaw() (restore func(), ok bool) {
	outMode, outOK := consoleMode(os.Stdout.Fd())
	inMode, inOK := consoleMode(os.Stdin.Fd())
	if !outOK || !inOK {
		return nil, false
	}
	if !setConsoleMode(os.Stdout.Fd(), outMode|enableVirtualTerminalProcessing) {
		return nil, false
	}
	if !setConsoleMode(os.Stdin.Fd(), inMode&^(enableLineInput|enableEchoInput|enableProcessedInput)) {
		setConsoleMode(os.Stdout.Fd(), outMode)
		return nil, false
	}
	return func() {
		setConsoleMode(os.Stdout.Fd(), outMode)
		setConsoleMode(os.Stdin.Fd(), inMode)
	}, true
}

func consoleMode(fd uintptr) (uint32, bool) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
	return mode, r != 0
}

func setConsoleMode(fd uintptr, mode uint32) bool {
	r, _, _ := procSetConsoleMode.Call(fd, uintptr(mode))
	return r != 0
}
