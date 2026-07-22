package cmd

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

var (
	initConfigFile string
	initAccessible bool
	initForce      bool
)

type initCommandOptions struct {
	configPath string
	accessible bool
	force      bool
}

type initCommandDependencies struct {
	in      io.Reader
	out     io.Writer
	entropy io.Reader
	getenv  func(string) string
	wizard  func(*initAnswers, initWizardOptions) error
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new Chatto server configuration",
	Long: `Create a new Chatto server configuration with an interactive setup wizard.

The wizard chooses the public address and NATS topology, generates fresh
secrets, and writes a private chatto.toml. Existing files are protected unless
you explicitly pass --force and confirm the overwrite in the wizard.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		configPath := initConfigFile
		if configPath == "" {
			configPath = "chatto.toml"
		}
		return runInitCommand(initCommandOptions{
			configPath: configPath,
			accessible: initAccessible,
			force:      initForce,
		}, initCommandDependencies{
			in:      cmd.InOrStdin(),
			out:     cmd.OutOrStdout(),
			entropy: rand.Reader,
			getenv:  os.Getenv,
			wizard:  runInitWizard,
		})
	},
}

func runInitCommand(opts initCommandOptions, deps initCommandDependencies) error {
	if _, err := os.Stat(opts.configPath); err == nil {
		if !opts.force {
			return fmt.Errorf("configuration already exists at %s; refusing to overwrite it (use --force to replace it)", opts.configPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect configuration path %s: %w", opts.configPath, err)
	}

	answers := defaultInitAnswers()
	accessible := opts.accessible ||
		deps.getenv("CHATTO_ACCESSIBLE") != "" ||
		strings.EqualFold(strings.TrimSpace(deps.getenv("TERM")), "dumb")
	if err := deps.wizard(&answers, initWizardOptions{
		input:      deps.in,
		output:     deps.out,
		accessible: accessible,
		configPath: opts.configPath,
		force:      opts.force,
	}); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("setup cancelled; nothing was written")
		}
		return fmt.Errorf("run setup wizard: %w", err)
	}
	if !answers.Confirmed {
		return errors.New("setup cancelled; nothing was written")
	}

	cfg, err := buildInitialConfig(answers, deps.entropy)
	if err != nil {
		return fmt.Errorf("build configuration: %w", err)
	}
	contents, err := renderInitialConfig(cfg, answers.NATSMode)
	if err != nil {
		return fmt.Errorf("render configuration: %w", err)
	}
	if err := writePrivateFile(opts.configPath, []byte(contents), opts.force); err != nil {
		return err
	}

	fmt.Fprintf(deps.out, "\nConfiguration written to %s\n", opts.configPath)
	fmt.Fprintln(deps.out, "The lights are on. Start the conversation with:")
	fmt.Fprintf(deps.out, "  chatto run --config %s\n", opts.configPath)
	return nil
}

func writePrivateFile(path string, contents []byte, overwrite bool) error {
	if !overwrite {
		return writeNewPrivateFile(path, contents)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary configuration beside %s: %w", path, err)
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		_ = temp.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure temporary configuration for %s: %w", path, err)
	}
	if _, err := temp.Write(contents); err != nil {
		return fmt.Errorf("write temporary configuration for %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary configuration for %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary configuration for %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace configuration %s: %w", path, err)
	}
	removeTemp = false
	return nil
}

func writeNewPrivateFile(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("configuration already exists at %s; refusing to overwrite it", path)
		}
		return fmt.Errorf("create configuration %s: %w", path, err)
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(path)
		}
	}()

	if _, err := file.Write(contents); err != nil {
		return fmt.Errorf("write configuration %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync configuration %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration %s: %w", path, err)
	}
	removeOnFailure = false
	return nil
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVarP(&initConfigFile, "config", "c", "", "path to configuration file (default: chatto.toml)")
	initCmd.Flags().BoolVar(&initAccessible, "accessible", false, "use screen-reader-friendly prompts (also enabled by CHATTO_ACCESSIBLE)")
	initCmd.Flags().BoolVar(&initForce, "force", false, "replace an existing configuration after interactive confirmation")
}
