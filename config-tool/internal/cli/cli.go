// Package cli is the command-line front end of dsc-config; it calls the same core as the GUI.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/render"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
)

const (
	binaryName = "dsc-config"
	// stdoutPath as --output writes the values to standard output.
	stdoutPath    = "-"
	flagProfile   = "profile"
	flagOutput    = "output"
	flagNoBrowser = "no-browser"
	// outputFileMode is the mode of a new values file before the umask; it holds no secrets.
	outputFileMode = 0o644
)

// GUIOptions are the root command's settings for the GUI.
type GUIOptions struct {
	// NoBrowser only prints the URL instead of also opening a browser.
	NoBrowser bool
	Stdout    io.Writer
	Stderr    io.Writer
}

// GUIRunner starts the GUI and blocks until it is closed or ctx is cancelled.
type GUIRunner func(ctx context.Context, opts GUIOptions) error

// NewRootCommand builds the dsc-config command tree; runGUI runs when no subcommand is given.
func NewRootCommand(runGUI GUIRunner) *cobra.Command {
	var noBrowser bool
	root := &cobra.Command{
		Use:   binaryName,
		Short: "Guided configuration of the FIWARE Data Space Connector chart",
		Long: binaryName + " produces a values.yaml for the data-space-connector chart.\n" +
			"Without a subcommand it starts the GUI; the subcommands run headless.",
		Version:       fmt.Sprintf("%s (%s)", version.Version, version.Commit),
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runGUI(cmd.Context(), GUIOptions{NoBrowser: noBrowser, Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr()})
		},
	}
	root.Flags().BoolVar(&noBrowser, flagNoBrowser, false, "only print the URL of the GUI, do not open a browser")
	root.SetVersionTemplate(binaryName + " {{.Version}}\n")
	root.AddCommand(newGenerateCommand())
	return root
}

func newGenerateCommand() *cobra.Command {
	var profilePath, outputPath string
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Render a values.yaml from a profile, without the GUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Loading the profile file arrives with CT-4; rendering fails before it matters.
			values, err := render.Render(cmd.Context(), profile.Profile{})
			if err != nil {
				return fmt.Errorf("generate: %w", err)
			}
			return writeOutput(cmd.OutOrStdout(), outputPath, values)
		},
	}
	cmd.Flags().StringVarP(&profilePath, flagProfile, "p", "", "profile (.dscprofile.yaml) holding the answers")
	cmd.Flags().StringVarP(&outputPath, flagOutput, "o", stdoutPath, "file to write the values.yaml to, "+stdoutPath+" for stdout")
	_ = cmd.MarkFlagRequired(flagProfile)
	return cmd
}

// writeOutput writes data to stdout or to a new file at path; an existing path, symlinks
// included, is refused and left untouched, and a failed write removes the file it created.
func writeOutput(stdout io.Writer, path string, data []byte) error {
	if path == stdoutPath {
		_, err := stdout.Write(data)
		return err
	}
	// TODO(CT-2): the round-trip ticket decides whether and how an existing file may be replaced.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, outputFileMode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

// Execute runs dsc-config with the process arguments and returns the exit code.
func Execute(ctx context.Context) int {
	return run(ctx, NewRootCommand(runGUI), os.Args[1:], os.Stderr)
}

func run(ctx context.Context, root *cobra.Command, args []string, stderr io.Writer) int {
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", binaryName, err)
		return 1
	}
	return 0
}
