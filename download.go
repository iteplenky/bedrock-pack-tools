package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type downloadTracker struct {
	mu         sync.Mutex
	totalPacks int
	startTime  time.Time
	received   int64
	connected  bool

	keys          *keyStore
	cdnDownloaded atomic.Int32
	cdnWg         sync.WaitGroup
	outDir        string
	verbose       bool
	httpClient    *http.Client
}

func (d *downloadTracker) onPackStart(id uuid.UUID, version string, current, total int) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.totalPacks = total
	if current == 0 {
		d.connected = true
		fmt.Printf("  Connected! %d packs, downloading...\n", total)
	}
	return true
}

func (d *downloadTracker) onPacket(header packet.Header, payload []byte, src, dst net.Addr) {
	if d.verbose {
		dir := "S→C"
		if src.String() == "client" {
			dir = "C→S"
		}
		fmt.Printf("%s  [DEBUG] %s packet 0x%02x (%d bytes)\n", clearLine, dir, header.PacketID, len(payload))
	}

	switch header.PacketID {
	case packet.IDPlayStatus:
		d.mu.Lock()
		if !d.connected {
			d.connected = true
			fmt.Println("  Authenticated, loading packs...")
		}
		d.mu.Unlock()
	case packet.IDResourcePacksInfo:
		d.onResourcePacksInfo(payload)
	case packet.IDResourcePackChunkData:
		d.mu.Lock()
		d.received += int64(len(payload))
		elapsed := time.Since(d.startTime).Seconds()
		if elapsed > 0 {
			speed := float64(d.received) / elapsed / 1024
			fmt.Printf("%s  Downloading: %.1f MB (%.0f KB/s)", clearLine, float64(d.received)/1024/1024, speed)
		}
		d.mu.Unlock()
	}
}

func (d *downloadTracker) onResourcePacksInfo(payload []byte) {
	packs, err := parseResourcePacks(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		return
	}

	var cdnPacks []protocol.TexturePackInfo
	for _, tp := range packs {
		if d.verbose {
			fmt.Printf("  [DEBUG] Pack: %s v%s size=%d key=%q url=%q\n",
				tp.UUID, tp.Version, tp.Size, tp.ContentKey, tp.DownloadURL)
		}
		if tp.DownloadURL != "" {
			cdnPacks = append(cdnPacks, tp)
		}
	}

	d.mu.Lock()
	d.startTime = time.Now()
	d.totalPacks = len(packs)
	d.mu.Unlock()

	d.keys.merge(collectKeys(packs))

	for _, tp := range cdnPacks {
		d.cdnWg.Add(1)
		go func() {
			defer d.cdnWg.Done()
			d.downloadFromURL(tp)
		}()
	}
}

func (d *downloadTracker) downloadFromURL(tp protocol.TexturePackInfo) {
	uid := tp.UUID.String()
	fmt.Printf("%s  CDN download: %s v%s from %s\n", clearLine, uid, tp.Version, tp.DownloadURL)

	resp, err := d.httpClient.Get(tp.DownloadURL)
	if err != nil {
		fmt.Printf("  %s[ERR]%s CDN download failed: %v\n", colorRed, colorReset, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("  %s[ERR]%s CDN returned HTTP %d\n", colorRed, colorReset, resp.StatusCode)
		return
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("  %s[ERR]%s CDN read failed: %v\n", colorRed, colorReset, err)
		return
	}

	fmt.Printf("  %s[CDN]%s Downloaded %.1f KB\n", colorGreen, colorReset, float64(len(data))/1024)
	d.cdnDownloaded.Add(1)

	if d.outDir == "" {
		return
	}

	dirName := sanitizePackName(uid) + "_v" + tp.Version
	packDir := filepath.Join(d.outDir, dirName)

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err == nil {
		n, extractErr := extractZip(zr, packDir)
		if extractErr != nil {
			fmt.Printf("  %s[ERR]%s Extract failed: %v\n", colorRed, colorReset, extractErr)
		} else {
			fmt.Printf("  %s[OK]%s  %-50s (%d files)\n", colorCyan, colorReset, dirName, n)
		}
		return
	}

	outFile := filepath.Join(d.outDir, dirName+mcpackExt)
	if err := os.WriteFile(outFile, data, 0644); err != nil {
		fmt.Printf("  %s[ERR]%s Save failed: %v\n", colorRed, colorReset, err)
	} else {
		fmt.Printf("  %s[OK]%s  Saved as %s (%.1f KB)\n", colorCyan, colorReset, outFile, float64(len(data))/1024)
	}
}

func runDownload(args []string) error {
	verbose := false
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "-v" || a == "--verbose" {
			verbose = true
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) < 1 {
		fmt.Println(`Usage: bedrock-pack-tools download [-v] <server:port> [output-dir]

Connect to a Minecraft Bedrock server, download all resource packs, and
extract them to disk. Also saves encryption keys.

Flags:
  -v, --verbose   Show all packet IDs for debugging

The output directory will contain one folder per pack: Name_vVersion/
A keys file (server_keys.json) is also saved alongside.

Examples:
  bedrock-pack-tools download <server:port>
  bedrock-pack-tools download <server:port> ./packs/`)
		return errUsage
	}

	server := args[0]
	outDir := "."
	if len(args) > 1 {
		outDir = args[1]
	}
	keysFile := filepath.Join(outDir, sanitizeServerAddr(server)+keysSuffix)

	fmt.Println("\n  ┌─ Pack Downloader ─────────────────────────")
	fmt.Println("  │ Server: " + server)
	fmt.Println("  │ Output: " + outDir)
	fmt.Println("  │ Keys:   " + keysFile)
	fmt.Println("  └──────────────────────────────────────────")

	tokenSource, err := getTokenSource()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	tracker := &downloadTracker{
		keys:       newKeyStore(keysFile),
		outDir:     outDir,
		verbose:    verbose,
		startTime:  time.Now(),
		httpClient: &http.Client{Timeout: 2 * time.Minute},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

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

	if err != nil {
		tracker.cdnWg.Wait()

		keyCount := tracker.keys.count()
		cdnCount := int(tracker.cdnDownloaded.Load())

		if cdnCount > 0 {
			fmt.Printf("\n  Connection closed after %.1fs, but %d packs downloaded via CDN\n", elapsed.Seconds(), cdnCount)
			if keyCount > 0 {
				fmt.Printf("  Keys: %d -> %s\n", keyCount, keysFile)
			}
			fmt.Println()
			return nil
		}
		if keyCount > 0 {
			fmt.Printf("\n  Connection closed after %.1fs, but %d keys saved -> %s\n", elapsed.Seconds(), keyCount, keysFile)
			fmt.Println("  Packs could not be downloaded (server didn't complete handshake).")
			fmt.Println("  Use 'keys' command + local pack cache for this server.")
			return nil
		}
		return fmt.Errorf("connection to %s failed: %w", server, err)
	}
	defer conn.Close()

	packs := conn.ResourcePacks()
	fmt.Printf("  Downloaded %d packs (%.1f MB) in %.1fs\n\n",
		len(packs), float64(tracker.received)/1024/1024, elapsed.Seconds())

	fmt.Println("  Extracting...")

	keys := make(map[string]keyEntry)
	var saved, encrypted, plain int

	for _, pack := range packs {
		name := sanitizePackName(pack.Name())
		version := pack.Version()
		uid := pack.UUID().String()
		dirName := name + "_v" + version
		packDir := filepath.Join(outDir, dirName)

		if pack.Encrypted() || pack.ContentKey() != "" {
			encrypted++
			keys[uid] = keyEntry{
				Key:     pack.ContentKey(),
				Version: version,
				Name:    name,
			}
		} else {
			plain++
		}

		n, err := extractResourcePack(pack, packDir)
		if err != nil {
			fmt.Printf("  %s[ERR]%s  %s: %v\n", colorRed, colorReset, dirName, err)
			continue
		}
		fmt.Printf("  %s[OK]%s   %-50s (%d files)\n", colorCyan, colorReset, dirName, n)
		saved++
	}

	for uid, early := range tracker.keys.snapshot() {
		if _, exists := keys[uid]; !exists {
			keys[uid] = early
		}
	}

	fmt.Printf("\n  Saved: %d/%d packs (%d encrypted, %d plain)\n", saved, len(packs), encrypted, plain)

	if len(keys) > 0 {
		if err := saveKeys(keys, keysFile); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not save keys: %v\n", err)
		}
		fmt.Printf("  Keys: %d -> %s\n", len(keys), keysFile)
	}
	fmt.Println()
	return nil
}
