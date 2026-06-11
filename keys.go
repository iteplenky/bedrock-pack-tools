package main

import (
	"context"
	"fmt"
	"maps"
	"net"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
	"github.com/iteplenky/gophertunnel/minecraft"
	"github.com/iteplenky/gophertunnel/minecraft/protocol/packet"
)

type keysTracker struct {
	mu         sync.Mutex
	totalPacks int
	keys       *keyStore
	cancel     context.CancelFunc

	connectSpinner *spinner
}

func (p *keysTracker) onPackStart(id uuid.UUID, version string, current, total int) bool {
	p.mu.Lock()
	p.totalPacks = total
	p.mu.Unlock()
	if p.connectSpinner != nil {
		p.connectSpinner.stop("")
	}
	fmt.Printf(lang.T("keys.pack.skipped"), clearLine, current+1, total, id, version)
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
		fmt.Fprintf(os.Stderr, lang.T("keys.warn"), err)
		// Cancel anyway - ResourcePacksInfo arrives once per connection.
		if p.cancel != nil {
			p.cancel()
		}
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
		fmt.Println(lang.T("keys.usage"))
		return errUsage
	}

	server := args[0]
	outFile := sanitizeServerAddr(server) + keysSuffix
	if len(args) > 1 {
		outFile = args[1]
	}

	fmt.Println(lang.T("keys.header.title"))
	fmt.Println(lang.T("keys.header.server") + server)
	fmt.Println(lang.T("keys.header.output") + outFile)
	fmt.Println(lang.T("keys.header.rule"))

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()
	ctx, cancel := context.WithTimeout(sigCtx, dialTimeout(120*time.Second))
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
	tracker.connectSpinner = startSpinner(lang.T("keys.spinner.connecting") + server)
	start := time.Now()

	conn, err := dialer.DialContext(ctx, "raknet", server)
	elapsed := time.Since(start)
	tracker.connectSpinner.stop("")

	keys := tracker.keys.snapshot()
	keyCount := len(keys)

	tracker.mu.Lock()
	totalPacks := tracker.totalPacks
	tracker.mu.Unlock()

	if err != nil {
		if keyCount > 0 {
			fmt.Printf(lang.T("keys.partial.captured"), clearLine, elapsed.Seconds(), totalPacks)
			printKeys(keys)
			fmt.Printf(lang.T("keys.partial.saved"), keyCount, outFile)
			return errPartialResult
		}
		return fmt.Errorf(lang.T("keys.connect.failed"), server, err)
	}
	defer conn.Close()

	fmt.Printf(lang.T("keys.connected"), clearLine, elapsed.Seconds())

	printKeys(keys)
	fmt.Printf(lang.T("keys.total"), totalPacks, keyCount)

	if keyCount > 0 {
		fmt.Printf(lang.T("keys.saved"), keyCount, outFile)
	} else {
		fmt.Println(lang.T("keys.none") + outFile + lang.T("keys.none.tail"))
	}
	return nil
}

func printKeys(keys map[string]keyEntry) {
	for _, uid := range slices.Sorted(maps.Keys(keys)) {
		info := keys[uid]
		fmt.Printf(lang.T("keys.entry.head"), colorYellow, colorReset, uid, info.Version, info.Name)
		fmt.Printf(lang.T("keys.entry.key"), colorGreen, info.Key, colorReset)
	}
}
