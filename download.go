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
	"github.com/iteplenky/gophertunnel/minecraft"
	"github.com/iteplenky/gophertunnel/minecraft/protocol"
	"github.com/iteplenky/gophertunnel/minecraft/protocol/packet"
)

// httpStatusErr signals a non-200 CDN response. Retryable() lets the
// retry loop abort early on permanent codes (404, 403) instead of
// burning attempts on them.
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
	decryptedDir     = "decrypted"
)

// decryptOutBase is where a download's decrypted packs land, grouped by
// server: <baseDir>/decrypted/<server>. Keeps multiple servers' decrypted
// packs from mixing in one folder.
func decryptOutBase(baseDir, server string) string {
	return filepath.Join(baseDir, decryptedDir, sanitizeServerAddr(server))
}

// cdnInitialBackoff is var (not const) so tests can shrink it.
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

	// connectSpinner: stop is sync.Once-guarded so the post-Dial safety
	// stop is a no-op if a callback already stopped it.
	connectSpinner *spinner
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
		if d.connectSpinner != nil {
			d.connectSpinner.stop("")
		}
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
			if d.connectSpinner != nil {
				d.connectSpinner.stop("")
			}
			fmt.Println("  Authenticated, loading packs...")
		}
	case packet.IDResourcePacksInfo:
		d.onResourcePacksInfo(payload)
	case packet.IDResourcePackChunkData:
		d.mu.Lock()
		d.received += int64(len(payload))
		received := d.received
		elapsed := time.Since(d.startTime).Seconds()
		// Throttle redraw - chunks arrive at hundreds/sec on a fast link.
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
	// Bound concurrent CDN downloads to avoid rate-limits / self-DDoS.
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
	// No-op on the rename-to-final-path branch below; cleans up the
	// extract-to-dir branch where the tmp file isn't renamed.
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
		// TexturePackInfo has no human-readable name; pull it from
		// the extracted manifest so naming matches protocol downloads.
		if name := readPackName(packDir); name != "" {
			nicer := filepath.Join(d.outDir, sanitizePackName(name)+"_v"+tp.Version)
			if nicer != packDir {
				if _, statErr := os.Stat(nicer); os.IsNotExist(statErr) {
					if renameErr := os.Rename(packDir, nicer); renameErr == nil {
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

// fetchToTemp streams the URL body to a tmp file inside d.outDir (so
// the later os.Rename is on the same FS). Retries cdnMaxRetries times
// with exponential backoff; honours d.ctx for cancel.
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
		// 4xx (except 408/429) is permanent - abort instead of retrying.
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
	// Match the real Bedrock client's UA. franchise.go does the same.
	req.Header.Set("User-Agent", "libhttpclient/1.0.0.0")
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

// isZipFile checks for the PK\x03\x04 local-file signature. Catches
// CDN error pages served with HTTP 200 that would otherwise get saved
// as a bogus .mcpack.
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
	var verbose, decrypt bool
	fs := newFlagSet()
	fs.Bool(&verbose, "-v", "--verbose")
	fs.Bool(&decrypt, "-d", "--decrypt")
	args, err := fs.parse(args)
	if err != nil {
		return err
	}

	if len(args) < 1 {
		fmt.Println(`Usage: bedrock-pack-tools download [-v] [--decrypt] <server:port> [output-dir]

Connect to a Minecraft Bedrock server, download all resource packs, and
extract them to disk. Also saves encryption keys.

Flags:
  -v, --verbose   Show all packet IDs for debugging
  -d, --decrypt   Decrypt the packs right after downloading (one step)

The output directory will contain one folder per pack: Name_vVersion/
A keys file (server_keys.json) is also saved alongside.

Without --decrypt the downloaded packs are still encrypted; turn them into
editable directories with: bedrock-pack-tools decrypt --all <keys.json> <output-dir>

Examples:
  bedrock-pack-tools download <server:port>
  bedrock-pack-tools download --decrypt <server:port> ./packs/`)
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

	// Ctrl+C cancels DialContext and in-flight HTTP fetches.
	sigCtx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignal()
	ctx, cancel := context.WithTimeout(sigCtx, dialTimeout(5*time.Minute))
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
	tracker.connectSpinner = startSpinner("Connecting to " + server)
	start := time.Now()

	conn, err := dialer.DialContext(ctx, "raknet", server)
	elapsed := time.Since(start)
	tracker.connectSpinner.stop("")

	if err != nil {
		tracker.cdnWg.Wait()

		keyCount := tracker.keys.count()
		cdnCount := int(tracker.cdnDownloaded.Load())

		tracker.mu.Lock()
		total := tracker.totalPacks
		tracker.mu.Unlock()

		// Every announced pack arrived over CDN before the server closed the
		// connection - that's a complete download, not a partial one. Some
		// servers ship every pack by URL and never finish the spawn handshake
		// for a non-playing client.
		if total > 0 && cdnCount >= total {
			fmt.Printf("\n  Downloaded %d/%d packs via CDN.\n", cdnCount, total)
			if keyCount > 0 {
				fmt.Printf("  Keys: %d -> %s\n", keyCount, keysFile)
				fmt.Printf("  To decrypt:  bedrock-pack-tools decrypt --all %s %s\n", keysFile, outDir)
			} else {
				fmt.Println("  Packs are unencrypted - ready to use, no decryption needed.")
			}
			fmt.Println()
			return nil
		}

		if cdnCount > 0 {
			fmt.Printf("\n  Connection closed after %.1fs, but %d packs downloaded via CDN\n", elapsed.Seconds(), cdnCount)
			if keyCount > 0 {
				fmt.Printf("  Keys: %d -> %s\n", keyCount, keysFile)
				fmt.Printf("  To decrypt:  bedrock-pack-tools decrypt --all %s %s\n", keysFile, outDir)
			}
			fmt.Println()
			return errPartialResult
		}
		if keyCount > 0 {
			fmt.Printf("\n  Connection closed after %.1fs, but %d keys saved -> %s\n", elapsed.Seconds(), keyCount, keysFile)
			fmt.Println("  Packs could not be downloaded (server didn't complete handshake).")
			fmt.Println("  Use 'keys' command + local pack cache for this server.")
			return errPartialResult
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

	// Wait for in-flight CDN downloads spawned from onResourcePacksInfo.
	tracker.cdnWg.Wait()

	fmt.Println("  Extracting...")

	keys := make(map[string]keyEntry)
	var saved, encrypted, plain int
	usedDirs := make(map[string]bool)

	for _, pack := range packs {
		name := sanitizePackName(pack.Name())
		version := pack.Version()
		uid := pack.UUID().String()
		dirName := name + "_v" + version
		// Two packs can share a Name_vVersion; append the UUID so neither
		// folder silently overwrites the other.
		if usedDirs[dirName] {
			dirName = dirName + "_" + uid[:8]
		}
		usedDirs[dirName] = true
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
		if decrypt {
			fmt.Println()
			return decryptAll(keysFile, outDir, decryptOutBase(outDir, server))
		}
		fmt.Printf("  To decrypt:  bedrock-pack-tools decrypt --all %s %s\n", keysFile, outDir)
	}
	fmt.Println()
	return nil
}
