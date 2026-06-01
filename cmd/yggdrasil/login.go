package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/dakasa-yggdrasil/yggdrasil/internal/corecli"
)

// runLogin exchanges username/password for a session token against
// a yggdrasil-core and persists the result in ~/.yggdrasil/config.yaml
// under a named context. Equivalent of `kubectl config set-context`
// + `kubectl config use-context`, but integrated with the auth flow.
func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("server", "", "yggdrasil-core base URL (e.g. http://localhost:9080)")
	username := fs.String("username", "", "login identifier (collaborator slug)")
	password := fs.String("password", "", "password (omit to prompt interactively)")
	totp := fs.String("totp", "", "TOTP/authenticator code (for MFA-enabled accounts)")
	recovery := fs.String("recovery-code", "", "MFA recovery code (alternative to --totp)")
	contextName := fs.String("context", "", "context name (default: derived from host)")
	nonInteractive := fs.Bool("non-interactive", false, "fail instead of prompting for missing values")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, `yggdrasil login — exchange credentials for a session token

Usage:
  yggdrasil login --server <url> --username <slug|email> [--password <value>]
  yggdrasil login --server <url> --username <email> --totp <code>   # MFA accounts

The username matches the collaborator's email (case-insensitive) or slug.
For MFA-enabled accounts, pass --totp <code> (or --recovery-code <code>); in
an interactive terminal you'll be prompted for the code automatically.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *server == "" {
		if !*nonInteractive {
			if err := huh.NewInput().Title("Server URL").Value(server).Run(); err != nil {
				return err
			}
		}
		if *server == "" {
			return fmt.Errorf("--server is required")
		}
	}
	if *username == "" {
		if !*nonInteractive {
			if err := huh.NewInput().Title("Username").Value(username).Run(); err != nil {
				return err
			}
		}
		if *username == "" {
			return fmt.Errorf("--username is required")
		}
	}
	if *password == "" {
		if !*nonInteractive {
			if err := huh.NewInput().Title("Password").Value(password).EchoMode(huh.EchoModePassword).Run(); err != nil {
				return err
			}
		}
		if *password == "" {
			return fmt.Errorf("--password is required")
		}
	}

	client := corecli.NewClient(*server, "")
	ctx := context.Background()
	res, err := client.LoginMFA(ctx, *username, *password, *totp, *recovery)
	if err != nil {
		return err
	}
	if res.MFARequired {
		if *nonInteractive {
			return fmt.Errorf("MFA required (factors: %s) — re-run with --totp <code> or --recovery-code <code>", strings.Join(res.Factors, ", "))
		}
		code := ""
		if err := huh.NewInput().Title("Authenticator code (TOTP)").Value(&code).Run(); err != nil {
			return err
		}
		res, err = client.LoginMFA(ctx, *username, *password, strings.TrimSpace(code), "")
		if err != nil {
			return err
		}
		if res.MFARequired {
			return fmt.Errorf("MFA code rejected — login not completed")
		}
	}
	token, slug := res.Token, res.CollaboratorSlug

	cfg, path, err := corecli.Load()
	if err != nil {
		return err
	}
	name := strings.TrimSpace(*contextName)
	if name == "" {
		name = deriveContextName(*server)
	}
	cfg.SetContext(name, &corecli.Context{
		Server:       *server,
		Token:        token,
		Collaborator: firstNonEmpty(slug, *username),
	}, true)
	if err := corecli.Save(cfg); err != nil {
		return err
	}

	fmt.Printf("logged in as %s → context %q saved to %s\n", firstNonEmpty(slug, *username), name, path)
	return nil
}

func deriveContextName(server string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(server, "https://"), "http://")
	if idx := strings.IndexAny(s, "/:"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.ReplaceAll(s, ".", "-")
	if s == "" {
		s = "default"
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
