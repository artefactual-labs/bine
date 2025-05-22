package envcmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-logr/logr"
	"github.com/peterbourgon/ff/v4"

	"github.com/artefactual-labs/bine/bine"
	"github.com/artefactual-labs/bine/cmd/rootcmd"
)

type Config struct {
	*rootcmd.RootConfig
	Command *ff.Command
	Flags   *ff.FlagSet
	Shell   string
}

func New(parent *rootcmd.RootConfig) *Config {
	var cfg Config
	cfg.RootConfig = parent
	cfg.Flags = ff.NewFlagSet("env").SetParent(parent.Flags)
	cfg.Flags.StringVar(&cfg.Shell, 's', "shell", "", "Shell format (fish, bash, zsh, sh).")
	cfg.Command = &ff.Command{
		Name:      "env",
		Usage:     "bine env [FLAGS]",
		ShortHelp: "Output shell commands to set up the PATH system variable.",
		LongHelp: `This command outputs shell code to configure your PATH environment variable for
the current project. By running the output in your shell, you can temporarily
add the bine's bin directory to your PATH, making project binaries immediately
available.

The output is formatted to match the syntax requirements of common shells,
such as Bash, Zsh and Fish. You can specify your preferred shell using the
"--shell" or "-s" flag. If you don't provide this option, the command will try
to detect your shell from the SHELL environment variable. If detection fails,
it will output POSIX-compatible syntax (using the standard export command) by
default.

Examples:

# Bash, Zsh (process substitution)
source <(bine env --shell=bash)

# Fish (source command)
bine env --shell=fish | source

# POSIX shells (sh, dash, etc.)
eval "$(bine env)"
`,
		Flags: cfg.Flags,
		Exec:  cfg.Exec,
	}
	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)
	return &cfg
}

func (cfg *Config) Exec(ctx context.Context, _ []string) error {
	logger, _ := logr.FromContext(ctx)
	b, err := bine.NewWithOptions(
		bine.WithCacheDir(cfg.CacheDir),
		bine.WithLogger(logger),
		bine.WithGitHubAPIToken(cfg.GitHubAPIToken),
	)
	if err != nil {
		return err
	}

	shell := cfg.shell()
	pathExport := cfg.formatPathExport(b.BinDir, shell)

	_, err = fmt.Fprintln(cfg.Stdout, pathExport)

	return err
}

func (cfg *Config) shell() string {
	if cfg.Shell != "" {
		return cfg.Shell
	}

	// Try to detect shell from environment.
	shell := os.Getenv("SHELL")
	if shell != "" {
		shell = filepath.Base(shell)
		switch shell {
		case "fish":
			return "fish"
		case "bash", "zsh", "sh":
			return "bash"
		}
	}

	// Default to bash format.
	return "bash"
}

func (cfg *Config) formatPathExport(binDir, shell string) string {
	path := quote(binDir)
	switch shell {
	case "fish":
		return fmt.Sprintf("fish_add_path --path %s", path)
	default:
		return fmt.Sprintf("export PATH=%s:$PATH", path)
	}
}

var pattern *regexp.Regexp = regexp.MustCompile(`[^\w@%+=:,./-]`)

// quote returns a shell-escaped version of the given string.
// Using `al.essio.dev/pkg/shellescape“ as a reference.
func quote(s string) string {
	if len(s) == 0 {
		return "''"
	}

	if pattern.MatchString(s) {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}

	return s
}
