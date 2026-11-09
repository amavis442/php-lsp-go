//go:build !unix

package main

import "os"

// openStdin returns stdin as is: on Windows, closing the handle already ends
// a pending read.
func openStdin() *os.File { return os.Stdin }
