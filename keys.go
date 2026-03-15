package main

import (
	"context"
	"fmt"
	"maps"
	"net"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type keysTracker struct {
	mu         sync.Mutex
	totalPacks int
	keys       *keyStore
	cancel     context.CancelFunc
}

func (p *keysTracker) onPackStart(id uuid.UUID, version string, current, total int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.totalPacks = total
	fmt.Printf("%s  Pack %d/%d: %s v%s (skipped)", clearLine, current+1, total, id, version)
	return false
}

func (p *keysTracker) onPacket(header packet.Header, payload []byte, src, dst net.Addr) {
	if header.PacketID == packet.IDResourcePacksInfo {
		p.onResourcePacksInfo(payload)
	}
}

func (p *keysTracker) onResourcePacksInfo(payload []byte) {
	packs, err := parseResourcePacks(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		return
	}

	p.mu.Lock()
	p.totalPacks = len(packs)
	p.mu.Unlock()

	p.keys.merge(collectKeys(packs))

	if p.cancel != nil {
		p.cancel()
	}
}

func runKeys(args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: bedrock-pack-tools keys <server:port> [output.json]

Connect to a Minecraft Bedrock server and extract resource pack encryption
keys. Authenticates via Xbox Live device code flow (token cached locally).

Examples:
  bedrock-pack-tools keys <server:port>
  bedrock-pack-tools keys <server:port> server_keys.json`)
		return errUsage
	}

	server := args[0]
	outFile := sanitizeServerAddr(server) + keysSuffix
	if len(args) > 1 {
		outFile = args[1]
	}

	fmt.Println("\n  ┌─ Pack Key Dumper ─────────────────────────")
	fmt.Println("  │ Server: " + server)
	fmt.Println("  │ Output: " + outFile)
	fmt.Println("  └──────────────────────────────────────────")

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tracker := &keysTracker{
		keys:   newKeyStore(outFile),
		cancel: cancel,
	}

	dialer := minecraft.Dialer{
		TokenSource:          tokenSource,
		DownloadResourcePack: tracker.onPackStart,
		PacketFunc:           tracker.onPacket,
	}

	fmt.Println()
	fmt.Println("  Connecting to " + server + " ...")
	start := time.Now()

	conn, err := dialer.DialContext(ctx, "raknet", server)
	elapsed := time.Since(start)

	keys := tracker.keys.snapshot()
	keyCount := len(keys)

	tracker.mu.Lock()
	totalPacks := tracker.totalPacks
	tracker.mu.Unlock()

	if err != nil {
		if keyCount > 0 {
			fmt.Printf("%s  Keys captured in %.1fs (%d packs on server)\n\n", clearLine, elapsed.Seconds(), totalPacks)
			printKeys(keys)
			fmt.Printf("\n  Saved %d keys -> %s\n\n", keyCount, outFile)
			return nil
		}
		return fmt.Errorf("connection to %s failed: %w", server, err)
	}
	defer conn.Close()

	fmt.Printf("%s  Connected! (%.1fs)\n\n", clearLine, elapsed.Seconds())

	printKeys(keys)
	fmt.Printf("\n  Total: %d packs (%d encrypted)\n", totalPacks, keyCount)

	if keyCount > 0 {
		fmt.Printf("  Saved %d keys -> %s\n\n", keyCount, outFile)
	} else {
		fmt.Println("\n  No encryption keys found.")
	}
	return nil
}

func printKeys(keys map[string]keyEntry) {
	for _, uid := range slices.Sorted(maps.Keys(keys)) {
		info := keys[uid]
		fmt.Printf("  %s[ENC]%s %s v%s \"%s\"\n", colorYellow, colorReset, uid, info.Version, info.Name)
		fmt.Printf("        KEY: %s%s%s\n", colorGreen, info.Key, colorReset)
	}
}
