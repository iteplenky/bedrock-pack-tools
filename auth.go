package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iteplenky/gophertunnel/minecraft/auth"
	"golang.org/x/oauth2"
)

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
		fmt.Fprintf(os.Stderr, "Warning: could not resolve token cache path: %v\n", err)
		return
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not marshal token: %v\n", err)
		return
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save token: %v\n", err)
	}
}

// tokenSourceAnnounced keeps the "Auth: using cached Xbox token" line
// to one print per CLI invocation - featuredDownload calls getTokenSource
// then chains into runDownload which would otherwise announce again.
var tokenSourceAnnounced bool

func getTokenSource() (oauth2.TokenSource, error) {
	if t := loadToken(); t != nil {
		if !tokenSourceAnnounced && os.Getenv(quietAuthEnv) == "" {
			fmt.Println("  Auth: using cached Xbox token")
			tokenSourceAnnounced = true
		}
		return auth.RefreshTokenSource(t), nil
	}

	fmt.Println("  Auth: no cached token - starting Xbox Live device auth")
	fmt.Println("  A URL and code will appear - enter it in your browser.")
	fmt.Println()

	tok, err := auth.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("xbox auth: %w", err)
	}
	saveToken(tok)
	fmt.Println("  Auth: token saved")
	fmt.Println()
	tokenSourceAnnounced = true
	return auth.RefreshTokenSource(tok), nil
}
