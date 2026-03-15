package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sandertv/gophertunnel/minecraft/auth"
	"golang.org/x/oauth2"
)

const tokenFileName = ".xbox_token.json"

func tokenPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return tokenFileName
	}
	p := filepath.Join(dir, "bedrock-pack-tools")
	if err := os.MkdirAll(p, 0700); err != nil {
		return tokenFileName
	}
	return filepath.Join(p, tokenFileName)
}

func loadToken() *oauth2.Token {
	data, err := os.ReadFile(tokenPath())
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
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not marshal token: %v\n", err)
		return
	}
	if err := os.WriteFile(tokenPath(), data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save token: %v\n", err)
	}
}

func getTokenSource() (oauth2.TokenSource, error) {
	if t := loadToken(); t != nil {
		fmt.Println("  Auth: using cached Xbox token")
		return auth.RefreshTokenSource(t), nil
	}

	fmt.Println("  Auth: no cached token — starting Xbox Live device auth")
	fmt.Println("  A URL and code will appear — enter it in your browser.")
	fmt.Println()

	tok, err := auth.TokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("xbox auth: %w", err)
	}
	saveToken(tok)
	fmt.Println("  Auth: token saved")
	fmt.Println()
	return auth.RefreshTokenSource(tok), nil
}
