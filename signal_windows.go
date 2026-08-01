//go:build windows

package main

import "os"

// notifyResize is a no-op on Windows, which has no resize signal. The screen
// is measured once at startup.
func notifyResize(c chan<- os.Signal) {}
