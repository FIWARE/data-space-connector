//go:build unix

package cluster

import (
	"os"
	"syscall"
)

// openFlags opens without blocking, so a path swapped for a FIFO after the stat check returns at
// once and is refused by the re-check; regular files ignore O_NONBLOCK.
const openFlags = os.O_RDONLY | syscall.O_NONBLOCK
