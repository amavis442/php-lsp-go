//go:build unix

package main

import (
	"os"
	"syscall"
)

// openStdin returns stdin in non-blocking mode. os.Stdin reads with blocking
// syscalls, and closing it does not interrupt a read that is already waiting,
// so the server would hang after the client's exit notification. A
// non-blocking descriptor goes through Go's network poller, where Close does
// wake the pending read.
func openStdin() *os.File {
	if err := syscall.SetNonblock(syscall.Stdin, true); err != nil {
		return os.Stdin
	}
	return os.NewFile(uintptr(syscall.Stdin), "/dev/stdin")
}
