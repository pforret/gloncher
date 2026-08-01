//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyResize delivers a value on c whenever the terminal is resized.
func notifyResize(c chan<- os.Signal) {
	signal.Notify(c, syscall.SIGWINCH)
}
