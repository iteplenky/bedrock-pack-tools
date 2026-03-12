package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/auth"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"golang.org/x/oauth2"
)

func tokenPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".xbox_token.json"
	}
	p := filepath.Join(dir, "bedrock-pack-tools")
	os.MkdirAll(p, 0700)
	return filepath.Join(p, ".xbox_token.json")
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

func getTokenSource() oauth2.TokenSource {
	if t := loadToken(); t != nil {
		fmt.Println("  Auth: using cached Xbox token")
		return auth.RefreshTokenSource(t)
	}

	fmt.Println("  Auth: no cached token — starting Xbox Live device auth")
	fmt.Println("  A URL and code will appear — enter it in your browser.")
	fmt.Println()

	src := auth.TokenSource
	tok, err := src.Token()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Auth error: %v\n", err)
		os.Exit(1)
	}
	saveToken(tok)
	fmt.Println("  Auth: token saved")
	fmt.Println()
	return auth.RefreshTokenSource(tok)
}

type progressTracker struct {
	mu            sync.Mutex
	totalPacks    int
	currentPack   int
	bytesReceived int64
	startTime     time.Time
}

func (p *progressTracker) onPackStart(id uuid.UUID, version string, current, total int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalPacks = total
	p.currentPack = current + 1
	fmt.Printf("\r\033[K  Pack %d/%d: %s v%s", p.currentPack, p.totalPacks, id, version)
	return true
}

func (p *progressTracker) onPacket(header packet.Header, payload []byte, src, dst net.Addr) {
	switch header.PacketID {
	case packet.IDResourcePacksInfo:
		p.mu.Lock()
		p.startTime = time.Now()
		p.mu.Unlock()
	case packet.IDResourcePackChunkData:
		p.mu.Lock()
		p.bytesReceived += int64(len(payload))
		elapsed := time.Since(p.startTime).Seconds()
		if elapsed > 0 {
			speed := float64(p.bytesReceived) / elapsed / 1024
			fmt.Printf("\r\033[K  Pack %d/%d: downloading... %.1f KB (%.0f KB/s)",
				p.currentPack, p.totalPacks, float64(p.bytesReceived)/1024, speed)
		}
		p.mu.Unlock()
	}
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

func runKeys(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage: bedrock-pack-tools keys <server:port> [output.json]

Connect to a Minecraft Bedrock server and extract resource pack encryption
keys. Authenticates via Xbox Live device code flow (token cached locally).

Examples:
  bedrock-pack-tools keys play.example.net:19132
  bedrock-pack-tools keys play.example.net:19132 server_keys.json`)
		os.Exit(1)
	}

	server := args[0]
	outFile := sanitize(server) + "_keys.json"
	if len(args) > 1 {
		outFile = args[1]
	}

	fmt.Println("\n  ┌─ Pack Key Dumper ─────────────────────────")
	fmt.Println("  │ Server: " + server)
	fmt.Println("  │ Output: " + outFile)
	fmt.Println("  └──────────────────────────────────────────")

	tokenSource := getTokenSource()

	progress := &progressTracker{startTime: time.Now()}

	dialer := minecraft.Dialer{
		TokenSource:          tokenSource,
		DownloadResourcePack: progress.onPackStart,
		PacketFunc:           progress.onPacket,
	}

	fmt.Println()
	fmt.Println("  Connecting to " + server + " ...")
	start := time.Now()

	conn, err := dialer.DialTimeout("raknet", server, 120*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n  Connection error: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	elapsed := time.Since(start)
	fmt.Printf("\r\033[K  Connected! (%.1fs, %.1f KB downloaded)\n\n",
		elapsed.Seconds(), float64(progress.bytesReceived)/1024)

	packs := conn.ResourcePacks()
	keys := make(map[string]keyEntry)
	encrypted, plain := 0, 0

	for _, pack := range packs {
		uid := pack.UUID()
		ver := pack.Version()
		name := pack.Name()
		key := pack.ContentKey()

		if pack.Encrypted() || key != "" {
			encrypted++
			fmt.Printf("  \033[33m[ENC]\033[0m %s v%s \"%s\"\n", uid, ver, name)
			fmt.Printf("        KEY: \033[32m%s\033[0m\n", key)
			keys[uid.String()] = keyEntry{
				Key:     key,
				Version: ver,
				Name:    name,
			}
		} else {
			plain++
			fmt.Printf("  \033[36m[---]\033[0m %s v%s \"%s\"\n", uid, ver, name)
		}
	}

	fmt.Printf("\n  Total: %d packs (%d encrypted, %d plain)\n", len(packs), encrypted, plain)

	if len(keys) > 0 {
		data, err := json.MarshalIndent(keys, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error marshalling keys: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(outFile, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "  Error writing: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("  Saved %d keys -> %s\n\n", len(keys), outFile)
	} else {
		fmt.Println("\n  No encryption keys found.")
	}
}
