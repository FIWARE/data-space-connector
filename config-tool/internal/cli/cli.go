// Package cli is the command-line front end of dsc-config; it calls the same core as the GUI.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/FIWARE/data-space-connector/config-tool/internal/atomicfile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/model"
	"github.com/FIWARE/data-space-connector/config-tool/internal/profile"
	"github.com/FIWARE/data-space-connector/config-tool/internal/roundtrip"
	"github.com/FIWARE/data-space-connector/config-tool/internal/trial"
	"github.com/FIWARE/data-space-connector/config-tool/internal/version"
	bundles "github.com/FIWARE/data-space-connector/config-tool/model"
	trialdata "github.com/FIWARE/data-space-connector/config-tool/trial"
)

const (
	binaryName = "dsc-config"
	// stdoutPath as --output writes the values to standard output.
	stdoutPath    = "-"
	flagProfile   = "profile"
	flagOutput    = "output"
	flagNoBrowser = "no-browser"
	flagTrial     = "trial"
	flagOutputDir = "output-dir"
	// quickStartURL is the Quick Start guide that installs the trial releases.
	quickStartURL = "https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/local-deployment/LOCAL.MD"
	warningPrefix = "warning: "
)

// errGeneratePending answers a valid generate until rendering is wired to the command.
var errGeneratePending = errors.New("rendering a values.yaml is not implemented yet")

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
	var profilePath, outputPath, outputDir string
	var withTrial bool
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Render a values.yaml from a profile, or the local trial's three, without the GUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if withTrial {
				return generateTrial(cmd.OutOrStdout(), cmd.ErrOrStderr(), outputDir)
			}
			if _, err := loadProfile(profilePath, bundles.FS); err != nil {
				return fmt.Errorf("generate: %w", err)
			}
			return fmt.Errorf("generate: %w", errGeneratePending)
		},
	}
	cmd.Flags().StringVarP(&profilePath, flagProfile, "p", "", "profile (.dscprofile.yaml) holding the answers")
	cmd.Flags().StringVarP(&outputPath, flagOutput, "o", stdoutPath, "file to write the values.yaml to, "+stdoutPath+" for stdout")
	cmd.Flags().BoolVar(&withTrial, flagTrial, false, "write the local trial's trust-anchor, provider and consumer values files")
	cmd.Flags().StringVar(&outputDir, flagOutputDir, ".", "directory to write the --"+flagTrial+" values files to")
	cmd.MarkFlagsOneRequired(flagProfile, flagTrial)
	cmd.MarkFlagsMutuallyExclusive(flagTrial, flagProfile)
	cmd.MarkFlagsMutuallyExclusive(flagTrial, flagOutput)
	cmd.MarkFlagsMutuallyExclusive(flagProfile, flagOutputDir)
	return cmd
}

// generateTrial writes the local trial's values files to dir, never overwriting. Every production
// warning goes to stderr first, so a refused or failed write still shows them.
func generateTrial(stdout, stderr io.Writer, dir string) error {
	for _, w := range trial.Warnings() {
		msg := warningPrefix + w.Message
		if w.Link != "" {
			msg += " See " + w.Link
		}
		fmt.Fprintln(stderr, msg)
	}
	t, err := trial.Load(trialdata.FS)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	outs, err := trial.Render(t, trialdata.FS, version.Version)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	paths, err := trial.WriteAll(dir, outs)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	for _, p := range paths {
		fmt.Fprintln(stdout, p)
	}
	fmt.Fprintln(stdout, "Install them as the Quick Start does: "+quickStartURL)
	return nil
}

// loadProfile reads the profile at path and refuses it unless it is complete enough to render:
// beyond the structural checks of save and open, every Target field must be answered and well-formed.
func loadProfile(path string, models fs.FS) (profile.Profile, error) {
	p, err := profile.ReadFile(path)
	if err != nil {
		return profile.Profile{}, err
	}
	shipped, err := model.Versions(models)
	if err != nil {
		return profile.Profile{}, err
	}
	errs := p.Validate(shipped)
	for _, e := range p.Target.Validate() {
		e.Field = profile.TargetFieldPrefix + e.Field
		if !slices.Contains(errs, e) {
			errs = append(errs, e)
		}
	}
	if len(errs) == 0 {
		return p, nil
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return profile.Profile{}, fmt.Errorf("%s: %w", path, &invalidProfileError{msgs})
}

// invalidProfileError lists every field error of a profile generate refuses.
type invalidProfileError struct {
	msgs []string
}

func (e *invalidProfileError) Error() string {
	return "invalid profile: " + strings.Join(e.msgs, "; ")
}

// outputExistsError refuses an existing output file; it matches fs.ErrExist.
type outputExistsError struct {
	path string
}

func (e *outputExistsError) Error() string {
	return e.path + " exists; merging into an existing values file arrives with the render epic"
}

func (e *outputExistsError) Unwrap() error { return fs.ErrExist }

// writeOutput writes data to stdout or to a new file at path; an existing path, symlinks
// included, is refused and left untouched, and a failed write removes the file it created.
func writeOutput(stdout io.Writer, path string, data []byte) error {
	if path == stdoutPath {
		_, err := stdout.Write(data)
		return err
	}
	// TODO(render): replace an existing file through replaceOutput once the render epic decides
	// (a) the precedence: overrides win, only owned or absent keys are written (FEATURE_MODEL,
	// ADR 0014), so a blind roundtrip.Merge is wrong; and (b) how the rendered output reaches
	// Merge: Render returns a map[string]any, or its bytes are decoded with yaml.v3 into one.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, roundtrip.ValuesFileMode)
	if errors.Is(err, fs.ErrExist) {
		return &outputExistsError{path}
	}
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

// replaceOutput applies edit to the existing values file at path and replaces it atomically,
// keeping its mode and every byte edit does not touch (ADR 0013). A symlink is refused.
func replaceOutput(path string, edit func(*roundtrip.Document) error) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s: %w", path, atomicfile.ErrSymlink)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	doc, err := roundtrip.Parse(src)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := edit(doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	out, err := doc.Write()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return atomicfile.Write(path, out, roundtrip.ValuesFileMode, true)
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
