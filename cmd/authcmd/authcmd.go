package authcmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/browser"
	"github.com/peterbourgon/ff/v4"
	"golang.org/x/oauth2"

	"github.com/artefactual-labs/bine/cmd/rootcmd"
	"github.com/artefactual-labs/bine/internal/auth"
)

const defaultHost = "github.com"

type authManager interface {
	Providers() []auth.Provider
	BeginDeviceAuth(context.Context, string) (auth.Provider, *oauth2.DeviceAuthResponse, error)
	CompleteDeviceAuth(context.Context, auth.Provider, *oauth2.DeviceAuthResponse) (auth.Credential, error)
	Status(context.Context, string) (auth.Credential, error)
	Logout(string) error
}

// Config configures the auth command and its subcommands.
type Config struct {
	*rootcmd.RootConfig
	Command   *ff.Command
	Flags     *ff.FlagSet
	manager   authManager
	openURL   func(string) error
	noBrowser bool
}

// New adds the auth command to parent.
func New(parent *rootcmd.RootConfig) *Config {
	cfg := Config{
		RootConfig: parent,
		manager:    parent.Auth,
		openURL:    browser.OpenURL,
	}
	cfg.Flags = ff.NewFlagSet("auth").SetParent(parent.Flags)
	cfg.Command = &ff.Command{
		Name:      "auth",
		Usage:     "bine auth <SUBCOMMAND>",
		ShortHelp: "Manage authentication for external services.",
		Flags:     cfg.Flags,
		Exec:      cfg.Exec,
	}

	loginFlags := ff.NewFlagSet("login").SetParent(cfg.Flags)
	loginFlags.BoolVar(&cfg.noBrowser, 0, "no-browser", "Do not try to open a web browser.")
	statusFlags := ff.NewFlagSet("status").SetParent(cfg.Flags)
	logoutFlags := ff.NewFlagSet("logout").SetParent(cfg.Flags)
	cfg.Command.Subcommands = append(cfg.Command.Subcommands,
		&ff.Command{
			Name:      "login",
			Usage:     "bine auth login [FLAGS] [HOST]",
			ShortHelp: "Authenticate with a service.",
			Flags:     loginFlags,
			Exec:      cfg.ExecLogin,
		},
		&ff.Command{
			Name:      "status",
			Usage:     "bine auth status [HOST]",
			ShortHelp: "Show saved authentication status.",
			Flags:     statusFlags,
			Exec:      cfg.ExecStatus,
		},
		&ff.Command{
			Name:      "logout",
			Usage:     "bine auth logout [HOST]",
			ShortHelp: "Remove saved authentication.",
			Flags:     logoutFlags,
			Exec:      cfg.ExecLogout,
		},
	)

	cfg.RootConfig.Command.Subcommands = append(cfg.RootConfig.Command.Subcommands, cfg.Command)

	return &cfg
}

// Exec reports that auth requires a subcommand.
func (cfg *Config) Exec(context.Context, []string) error {
	return errors.New("auth command requires a subcommand (login, status, logout)")
}

// ExecLogin authenticates with a service using its device flow.
func (cfg *Config) ExecLogin(ctx context.Context, args []string) error {
	host, err := hostArg(args)
	if err != nil {
		return err
	}

	provider, response, err := cfg.manager.BeginDeviceAuth(ctx, host)
	if err != nil {
		return err
	}

	fmt.Fprintf(cfg.Stdout, "Your one-time code is: %s\n", response.UserCode)
	fmt.Fprintf(cfg.Stdout, "Open %s to authorize Bine.\n", response.VerificationURI)
	if !cfg.noBrowser {
		if err := cfg.openURL(response.VerificationURI); err != nil {
			fmt.Fprintf(cfg.Stderr, "Could not open a browser: %v\n", err)
		}
	}
	fmt.Fprintln(cfg.Stdout, "Waiting for authorization...")

	credential, err := cfg.manager.CompleteDeviceAuth(ctx, provider, response)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(cfg.Stdout, "Logged in to %s as %s.\n", provider.Host, credential.Account)

	return err
}

// ExecStatus reports authentication status for one or all providers.
func (cfg *Config) ExecStatus(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return errors.New("auth status accepts at most one host")
	}

	if len(args) == 1 {
		return cfg.printStatus(ctx, args[0])
	}

	for _, provider := range cfg.manager.Providers() {
		if err := cfg.printStatus(ctx, provider.Host); err != nil {
			return err
		}
	}

	return nil
}

// ExecLogout removes authentication for a service.
func (cfg *Config) ExecLogout(_ context.Context, args []string) error {
	host, err := hostArg(args)
	if err != nil {
		return err
	}

	if err := cfg.manager.Logout(host); err != nil {
		if errors.Is(err, auth.ErrCredentialNotFound) {
			_, writeErr := fmt.Fprintf(cfg.Stdout, "Not logged in to %s.\n", host)
			return writeErr
		}
		return err
	}

	_, err = fmt.Fprintf(cfg.Stdout, "Logged out of %s.\n", host)

	return err
}

func (cfg *Config) printStatus(ctx context.Context, host string) error {
	credential, err := cfg.manager.Status(ctx, host)
	if err != nil {
		if errors.Is(err, auth.ErrCredentialNotFound) {
			_, writeErr := fmt.Fprintf(cfg.Stdout, "%s: not logged in\n", host)
			return writeErr
		}
		return err
	}

	_, err = fmt.Fprintf(cfg.Stdout, "%s: logged in as %s\n", host, credential.Account)

	return err
}

func hostArg(args []string) (string, error) {
	switch len(args) {
	case 0:
		return defaultHost, nil
	case 1:
		return args[0], nil
	default:
		return "", errors.New("auth command accepts at most one host")
	}
}
