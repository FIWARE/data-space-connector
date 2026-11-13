// Package version holds the build identity of dsc-config, stamped through -ldflags at build time.
package version

// Version is the release version; the Makefile sets it from the config-tool/v* tag.
var Version = "dev"

// Commit is the short git commit the binary was built from.
var Commit = "unknown"
