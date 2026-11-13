// Command dsc-config is the configuration tool of the FIWARE Data Space Connector.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/FIWARE/data-space-connector/config-tool/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Execute(ctx)
	stop()
	os.Exit(code)
}
