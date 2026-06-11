package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
	"github.com/iteplenky/gophertunnel/minecraft/auth"
	"golang.org/x/oauth2"
)

// runLogin ensures a cached Xbox token exists, starting the device-code flow
// when there isn't one. Exposed as the `login` subcommand and re-execed by the
// interactive menu (which hands it the real terminal for the device prompt).
func runLogin([]string) error {
	if _, err := getTokenSource(); err != nil {
		return err
	}
	fmt.Println(lang.T("auth.login.done"))
	return nil
}

// runLogout removes the cached Xbox + franchise tokens (the `logout`
// subcommand). The device.ID cohort is left alone - that's not a credential.
func runLogout([]string) error {
	if err := clearAuthCaches(); err != nil {
		return err
	}
	fmt.Println(lang.T("auth.logout.done"))
	return nil
}

// clearAuthCaches deletes the Xbox and MCToken caches. A missing file is not
// an error. The device.ID is deliberately not touched here.
func clearAuthCaches() error {
	var errs []error
	for _, resolve := range []func() (string, error){tokenPath, mctokenPath} {
		path, err := resolve()
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

const tokenFileName = ".xbox_token.json"

// quietAuthEnv suppresses the "using cached Xbox token" line. The
// interactive menu sets it (and passes it to its child processes) so the
// note doesn't linger on the main screen after the alt-screen exits. The
// device-code prompt still prints - that one the user must see.
const quietAuthEnv = "BPT_QUIET_AUTH"

// tokenPath returns the on-disk cache path for the Xbox token. Errors
// instead of falling back to cwd - the refresh token is the most
// sensitive artifact and must never leak into a shared working dir.
func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	p := filepath.Join(dir, "bedrock-pack-tools")
	if err := os.MkdirAll(p, 0700); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", p, err)
	}
	return filepath.Join(p, tokenFileName), nil
}

func loadToken() *oauth2.Token {
	path, err := tokenPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var t oauth2.Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil
	}
	if t.AccessToken == "" || t.RefreshToken == "" {
		return nil
	}
	return &t
}

func saveToken(t *oauth2.Token) {
	path, err := tokenPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.token.resolve"), err)
		return
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.token.marshal"), err)
		return
	}
	// Atomic write (tmp + rename) so a crash can't leave a truncated refresh
	// token, and an existing looser-mode file gets retightened.
	if err := atomicWriteFile(path, ".xbox_token-*.tmp", data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, lang.T("auth.warn.token.save"), err)
	}
}

// tokenSourceAnnounced keeps the "Auth: using cached Xbox token" line
// to one print per CLI invocation - featuredDownload calls getTokenSource
// then chains into runDownload which would otherwise announce again.
var tokenSourceAnnounced bool

func getTokenSource() (oauth2.TokenSource, error) {
	if t := loadToken(); t != nil {
		if !tokenSourceAnnounced && os.Getenv(quietAuthEnv) == "" {
			fmt.Println(lang.T("auth.cached"))
			tokenSourceAnnounced = true
		}
		return auth.RefreshTokenSource(t), nil
	}

	fmt.Println(lang.T("auth.start"))
	fmt.Println(lang.T("auth.prompt.hint"))
	fmt.Println()

	tok, err := auth.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("xbox auth: %w", err)
	}
	saveToken(tok)
	fmt.Println(lang.T("auth.saved"))
	fmt.Println()
	tokenSourceAnnounced = true
	return auth.RefreshTokenSource(tok), nil
}
