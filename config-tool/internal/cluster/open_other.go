//go:build !unix

package cluster

import "os"

// openFlags is a plain read-only open; these systems have no FIFOs in the file system.
const openFlags = os.O_RDONLY
