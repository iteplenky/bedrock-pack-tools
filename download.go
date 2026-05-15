package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// httpStatusErr is returned by fetchOnce when the server replies with a
// non-200 status. Retryable() distinguishes transient failures (5xx,
// 408, 429) from permanent ones (404, 403, ...) so the retry loop can
// give up immediately on the latter.
type httpStatusErr struct{ code int }

func (e *httpStatusErr) Error() string { return fmt.Sprintf("HTTP %d", e.code) }
func (e *httpStatusErr) Retryable() bool {
	return e.code == http.StatusRequestTimeout ||
		e.code == http.StatusTooManyRequests ||
		e.code >= 500
}

const (
	cdnConcurrency   = 6
	cdnMaxRetries    = 3
	progressThrottle = 100 * time.Millisecond
)

// cdnInitialBackoff is var, not const, so tests can shrink it from 500ms
// to ~1ms and still exercise the retry path in milliseconds.
var cdnInitialBackoff = 500 * time.Millisecond

type downloadTracker struct {
	mu           sync.Mutex
	totalPacks   int
	startTime    time.Time
	received     int64
	connected    bool
	lastProgress time.Time

	keys          *keyStore
	cdnDownloaded atomic.Int32
	cdnWg         sync.WaitGroup
	cdnSem        chan struct{}
	ctx           context.Context
	outDir        string
	verbose       bool
	httpClient    *http.Client
}

func (d *downloadTracker) onPackStart(id uuid.UUID, version string, current, total int) bool {
	d.mu.Lock()
	d.totalPacks = total
	firstPack := !d.connected && current == 0
	if firstPack {
		d.connected = true
	}
	d.mu.Unlock()
	if firstPack {
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
		announce := !d.connected
		if announce {
			d.connected = true
		}
		d.mu.Unlock()
		if announce {
			fmt.Println("  Authenticated, loading packs...")
		}
	case packet.IDResourcePacksInfo:
		d.onResourcePacksInfo(payload)
	case packet.IDResourcePackChunkData:
		d.mu.Lock()
		d.received += int64(len(payload))
		received := d.received
		elapsed := time.Since(d.startTime).Seconds()
		// Chunk packets arrive at hundreds-per-second on a fast link;
		// throttle the redraw so printf+lock contention doesn't dominate.
		shouldPrint := time.Since(d.lastProgress) >= progressThrottle
		if shouldPrint {
			d.lastProgress = time.Now()
		}
		d.mu.Unlock()
		if shouldPrint && elapsed > 0 {
			speed := float64(received) / elapsed / 1024
			fmt.Printf("%s  Downloading: %.1f MB (%.0f KB/s)", clearLine, float64(received)/1024/1024, speed)
		}
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
		d.cdnWg.Go(func() { d.downloadFromURL(tp) })
	}
}

func (d *downloadTracker) downloadFromURL(tp protocol.TexturePackInfo) {
	// Cap parallel CDN downloads — without this a server advertising 50
	// CDN packs would spawn 50 simultaneous HTTP requests and risk a
	// rate-limit (or self-DDoS the CDN endpoint).
	select {
	case d.cdnSem <- struct{}{}:
		defer func() { <-d.cdnSem }()
	case <-d.ctx.Done():
		return
	}

	uid := tp.UUID.String()
	fmt.Printf("%s  CDN download: %s v%s from %s\n", clearLine, uid, tp.Version, tp.DownloadURL)

	tmpPath, size, err := d.fetchToTemp(tp.DownloadURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  %s[ERR]%s CDN download failed: %v\n", colorRed, colorReset, err)
		return
	}
	// If the body was streamed but not consumed by the extract/save branch
	// below, drop it. os.Remove on an already-renamed file is a no-op error.
	defer os.Remove(tmpPath)

	fmt.Printf("  %s[CDN]%s Downloaded %.1f KB\n", colorGreen, colorReset, float64(size)/1024)
	d.cdnDownloaded.Add(1)

	if d.outDir == "" {
		return
	}

	dirName := sanitizePackName(uid) + "_v" + tp.Version
	packDir := filepath.Join(d.outDir, dirName)

	if isZipFile(tmpPath) {
		f, err := os.Open(tmpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s[ERR]%s open tmp: %v\n", colorRed, colorReset, err)
			return
		}
		zr, zipErr := zip.NewReader(f, size)
		if zipErr != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "  %s[ERR]%s zip parse: %v\n", colorRed, colorReset, zipErr)
			return
		}
		n, extractErr := extractZip(zr, packDir)
		f.Close()
		if extractErr != nil {
			fmt.Fprintf(os.Stderr, "  %s[ERR]%s Extract failed: %v\n", colorRed, colorReset, extractErr)
			return
		}
		// TexturePackInfo carries no human-readable name; pull it from the
		// extracted manifest so the dir matches protocol-downloaded packs.
		if name := readPackName(packDir); name != "" {
			nicer := filepath.Join(d.outDir, sanitizePackName(name)+"_v"+tp.Version)
			if nicer != packDir {
				if _, statErr := os.Stat(nicer); os.IsNotExist(statErr) {
					if renameErr := os.Rename(packDir, nicer); renameErr == nil {
						packDir = nicer
						dirName = filepath.Base(nicer)
					}
				}
			}
		}
		fmt.Printf("  %s[OK]%s  %-50s (%d files)\n", colorCyan, colorReset, dirName, n)
		return
	}

	outFile := filepath.Join(d.outDir, dirName+mcpackExt)
	if err := os.Rename(tmpPath, outFile); err != nil {
		fmt.Fprintf(os.Stderr, "  %s[ERR]%s Save failed: %v\n", colorRed, colorReset, err)
		return
	}
	_ = os.Chmod(outFile, 0644)
	fmt.Printf("  %s[OK]%s  Saved as %s (%.1f KB)\n", colorCyan, colorReset, outFile, float64(size)/1024)
}

// fetchToTemp streams the URL's body to a temporary file in d.outDir
// (so the later os.Rename to the final location is on the same FS) and
// returns the temp path plus byte size. Retries cdnMaxRetries times with
// exponential backoff on transient errors; honours d.ctx for cancel.
func (d *downloadTracker) fetchToTemp(url string) (string, int64, error) {
	backoff := cdnInitialBackoff
	var lastErr error
	for attempt := 1; attempt <= cdnMaxRetries; attempt++ {
		if err := d.ctx.Err(); err != nil {
			return "", 0, err
		}
		path, size, err := d.fetchOnce(url)
		if err == nil {
			return path, size, nil
		}
		lastErr = err
		// 4xx (other than 408/429) is permanent — no point burning
		// further attempts and backoff on a 404 or 403.
		var hse *httpStatusErr
		if errors.As(err, &hse) && !hse.Retryable() {
			return "", 0, err
		}
		if attempt == cdnMaxRetries {
			break
		}
		select {
		case <-time.After(backoff):
		case <-d.ctx.Done():
			return "", 0, d.ctx.Err()
		}
		backoff *= 2
	}
	return "", 0, lastErr
}

func (d *downloadTracker) fetchOnce(url string) (string, int64, error) {
	req, err := http.NewRequestWithContext(d.ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", 0, &httpStatusErr{code: resp.StatusCode}
	}
	tmpDir := d.outDir
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	f, err := os.CreateTemp(tmpDir, "cdn-*.tmp")
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", 0, err
	}
	return f.Name(), n, nil
}

// isZipFile peeks the first 4 bytes for the standard zip local-file
// signature (PK\x03\x04). Without this, a CDN that returns an HTML
// error page with HTTP 200 would slip past zip.NewReader and get
// saved as a bogus .mcpack.
func isZipFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false
	}
	return magic == [4]byte{'P', 'K', 0x03, 0x04}
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

	// Honour Ctrl+C: cancel triggers DialContext + in-flight HTTP requests
	// (via http.NewRequestWithContext) so partial files don't linger.
	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()
	ctx, cancel := context.WithTimeout(sigCtx, 5*time.Minute)
	defer cancel()

	tracker := &downloadTracker{
		keys:       newKeyStore(keysFile),
		outDir:     outDir,
		verbose:    verbose,
		startTime:  time.Now(),
		httpClient: &http.Client{Timeout: 2 * time.Minute},
		cdnSem:     make(chan struct{}, cdnConcurrency),
		ctx:        ctx,
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

	tracker.mu.Lock()
	totalReceived := tracker.received
	tracker.mu.Unlock()

	fmt.Printf("  Downloaded %d packs (%.1f MB) in %.1fs\n\n",
		len(packs), float64(totalReceived)/1024/1024, elapsed.Seconds())

	// CDN downloads from onResourcePacksInfo run in their own goroutines.
	// Wait for them so the final summary reflects everything that landed,
	// and we don't kill in-flight transfers when main returns.
	tracker.cdnWg.Wait()

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
