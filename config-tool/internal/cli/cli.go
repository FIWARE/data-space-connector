// Package cli is the command-line front end of dsc-config; it calls the same core as the GUI.
package cli

import (
	"context"
	"errors"
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
	stdoutPath  = "-"
	flagProfile = "profile"
	flagOutput  = "output"
)

// ErrGUIUnavailable is returned by the GUI stub until the local server lands.
var ErrGUIUnavailable = errors.New("GUI not available yet")

// GUIRunner starts the GUI and blocks until it is closed or ctx is cancelled.
type GUIRunner func(ctx context.Context) error

// NewRootCommand builds the dsc-config command tree; runGUI runs when no subcommand is given.
func NewRootCommand(runGUI GUIRunner) *cobra.Command {
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
			return runGUI(cmd.Context())
		},
	}
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

func writeOutput(stdout io.Writer, path string, data []byte) error {
	if path == stdoutPath {
		_, err := stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Execute runs dsc-config with the process arguments and returns the exit code.
func Execute(ctx context.Context) int {
	return run(ctx, NewRootCommand(stubGUI), os.Args[1:], os.Stderr)
}

func run(ctx context.Context, root *cobra.Command, args []string, stderr io.Writer) int {
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", binaryName, err)
		return 1
	}
	return 0
}

func stubGUI(context.Context) error {
	return ErrGUIUnavailable
}
