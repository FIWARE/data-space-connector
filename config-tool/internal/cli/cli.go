// Package cli is the command-line front end of dsc-config; it calls the same core as the GUI.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	// outputFileMode is the mode of a new values file before the umask; it holds no secrets.
	outputFileMode = 0o644
	// stagingAttempts bounds the retries when a random scratch file name is already taken.
	stagingAttempts = 10
	// tempPattern names the scratch file the output is staged in before the rename.
	tempPattern = ".dsc-config-*.tmp"
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

// writeOutput writes data to stdout or, atomically, to path: a failure never leaves a partial file.
// A symlink at path is followed and its target replaced, keeping that file's mode; a new file
// gets outputFileMode minus the umask. Ownership is not carried over.
// CT-2: decide whether an existing file at path may be replaced; today it is.
func writeOutput(stdout io.Writer, path string, data []byte) error {
	if path == stdoutPath {
		_, err := stdout.Write(data)
		return err
	}
	target, existingMode, err := resolveOutput(path)
	if err != nil {
		return err
	}
	tmp, err := createStaged(filepath.Dir(target))
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if existingMode != nil {
		if err := tmp.Chmod(*existingMode); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

// resolveOutput follows symlinks at path and returns the file to replace and, if it exists, its mode.
func resolveOutput(path string) (string, *os.FileMode, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return path, nil, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", nil, err
	}
	mode := info.Mode().Perm()
	return target, &mode, nil
}

// createStaged creates a fresh scratch file in dir; unlike os.CreateTemp it lets the umask apply.
func createStaged(dir string) (*os.File, error) {
	for range stagingAttempts {
		name := filepath.Join(dir, strings.Replace(tempPattern, "*", strconv.FormatUint(rand.Uint64(), 36), 1))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, outputFileMode)
		if !errors.Is(err, fs.ErrExist) {
			return f, err
		}
	}
	return nil, fmt.Errorf("no free scratch file name in %s", dir)
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
