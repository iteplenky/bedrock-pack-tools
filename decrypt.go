package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/cfb8"
)

func decryptContentsJSON(data []byte, packKey string) (*contentsFile, error) {
	if len(data) < contentsHeaderSize {
		return nil, fmt.Errorf("%w: %d bytes", errPackTruncated, len(data))
	}
	encrypted := data[contentsHeaderSize:]
	plaintext, err := cfb8.Decrypt(encrypted, []byte(packKey))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errPackWrongKey, err)
	}
	plaintext = bytes.TrimRight(plaintext, "\x00 \n\r\t")

	var contents contentsFile
	if err := json.Unmarshal(plaintext, &contents); err != nil {
		preview := string(plaintext)
		if len(preview) > 100 {
			preview = preview[:100]
		}
		return nil, fmt.Errorf("%w: parse failed (first 100 bytes: %q): %w", errPackWrongKey, preview, err)
	}
	return &contents, nil
}

func runDecrypt(args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage:
  bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]

Decrypt a single encrypted resource pack:
  bedrock-pack-tools decrypt ./my_packs/SomePack_v1.0.0 YOUR_32_CHAR_KEY

Batch-decrypt all packs matched by a keys.json file:
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/ ./decrypted/`)
		return errUsage
	}

	if args[0] == "--all" {
		if len(args) < 3 {
			fmt.Println("Usage: bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]")
			return errUsage
		}
		outDir := ""
		if len(args) > 3 {
			outDir = args[3]
		}
		return decryptAll(args[1], args[2], outDir)
	}

	if len(args) < 2 {
		fmt.Println("Usage: bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]")
		return errUsage
	}
	outDir := strings.TrimRight(args[0], "/\\") + "_decrypted"
	if len(args) > 2 {
		outDir = args[2]
	}
	return decryptPack(args[0], args[1], outDir)
}

type packJob struct {
	name   string
	dir    string
	key    string
	outDir string
}

// defaultDecryptOutBase picks the sibling output dir for --all (avoids
// colliding with a real pack named "decrypted" inside packsDir).
func defaultDecryptOutBase(packsDir string) string {
	trimmed := strings.TrimRight(packsDir, "/\\")
	if trimmed == "" || trimmed == "." {
		return "decrypted"
	}
	return trimmed + "_decrypted"
}

func decryptAll(keysFile, packsDir, outBase string) error {
	data, err := os.ReadFile(keysFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", keysFile, err)
	}
	var keys map[string]keyEntry
	if err := json.Unmarshal(data, &keys); err != nil {
		return fmt.Errorf("parse %s: %w", keysFile, err)
	}

	if outBase == "" {
		outBase = defaultDecryptOutBase(packsDir)
	}

	entries, err := os.ReadDir(packsDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", packsDir, err)
	}

	var jobs []packJob
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		packDir := filepath.Join(packsDir, entry.Name())

		if _, err := os.Stat(filepath.Join(packDir, contentsJSON)); err != nil {
			continue
		}

		packUUID, err := readPackUUID(packDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  %s[WARN]%s %s - %v\n", colorYellow, colorReset, entry.Name(), err)
			continue
		}

		keyInfo, ok := keys[packUUID]
		if !ok {
			fmt.Printf("  %s[SKIP]%s %s - no key for UUID %s\n",
				colorYellow, colorReset, entry.Name(), packUUID)
			continue
		}

		jobs = append(jobs, packJob{
			name:   entry.Name(),
			dir:    packDir,
			key:    keyInfo.Key,
			outDir: filepath.Join(outBase, entry.Name()),
		})
	}

	if len(jobs) == 0 {
		fmt.Println("  No packs matched.")
		return nil
	}

	workers := min(runtime.NumCPU(), len(jobs))

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		jobCh     = make(chan packJob, len(jobs))
	)

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				stats, err := decryptPackInner(job.dir, job.key, job.outDir)

				mu.Lock()
				if err != nil {
					fmt.Printf("  %s[ERROR]%s %s: %v\n", colorRed, colorReset, job.name, err)
				} else {
					fmt.Printf("  %s[OK]%s %s (%d decrypted, %d copied, %d errors)\n",
						colorCyan, colorReset, job.name, stats.decrypted, stats.copied, stats.errors)
					succeeded++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	dest := outBase
	if abs, err := filepath.Abs(outBase); err == nil {
		dest = abs
	}
	fmt.Printf("\n  Decrypted %d/%d packs\n  Location: %s\n", succeeded, len(jobs), dest)
	if succeeded == 0 {
		return fmt.Errorf("all %d packs failed to decrypt", len(jobs))
	}
	return nil
}

func decryptPack(packDir, packKey, outDir string) error {
	fmt.Println()
	fmt.Println("  Pack:   " + packDir)
	fmt.Println("  Key:    " + packKey)
	fmt.Println("  Output: " + outDir)
	fmt.Println()

	stats, err := decryptPackInner(packDir, packKey, outDir)
	if err != nil {
		return err
	}
	fmt.Printf("  Done! %d decrypted, %d copied, %d errors\n",
		stats.decrypted, stats.copied, stats.errors)
	return nil
}

type packStats struct {
	decrypted int
	copied    int
	errors    int
}

type fileResult struct {
	path      string
	decrypted bool
	err       error
}

func decryptPackInner(packDir, packKey, outDir string) (packStats, error) {
	// Wrap missing contents.json in errPackNoManifest so humanize can
	// explain "this isn't a pack folder" instead of a bare open error.
	contentsPath := filepath.Join(packDir, contentsJSON)
	if _, err := os.Stat(contentsPath); os.IsNotExist(err) {
		return packStats{}, fmt.Errorf("%w: no contents.json at %s", errPackNoManifest, contentsPath)
	}
	contentsData, err := os.ReadFile(contentsPath)
	if err != nil {
		return packStats{}, fmt.Errorf("read contents.json: %w", err)
	}

	contents, err := decryptContentsJSON(contentsData, packKey)
	if err != nil {
		return packStats{}, err
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return packStats{}, fmt.Errorf("create output dir: %w", err)
	}

	cj, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return packStats{}, fmt.Errorf("marshal contents.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, contentsJSON), cj, 0644); err != nil {
		return packStats{}, fmt.Errorf("write contents.json: %w", err)
	}

	stats := decryptPackFiles(packDir, outDir, contents.Content)
	stats.copied++ // contents.json itself, written above

	// manifest.json and pack_icon.png are plain and some packs omit them
	// from contents.json - copy them across so the decrypted pack loads.
	for _, name := range []string{manifestJSON, packIconPNG} {
		if err := copyIfMissing(packDir, outDir, name); err != nil {
			fmt.Fprintf(os.Stderr, "    %s[ERR]%s %s: %v\n", colorRed, colorReset, name, err)
			stats.errors++
		}
	}

	return stats, nil
}

// decryptPackFiles decrypts or copies every entry in the pack's
// contents.json, fanning out across workers. contents.json itself is
// skipped - the caller writes it.
func decryptPackFiles(packDir, outDir string, entries []contentsEntry) packStats {
	type fileJob struct {
		entry   contentsEntry
		srcPath string
		dstPath string
	}

	// contents.json comes from a downloaded, untrusted pack. Refuse any entry
	// whose path escapes outDir (zip-slip), the same guard extractZip applies.
	cleanBase := filepath.Clean(outDir) + string(os.PathSeparator)
	var jobs []fileJob
	var escaped int
	for _, entry := range entries {
		if entry.Path == contentsJSON {
			continue
		}
		dstPath := filepath.Join(outDir, entry.Path)
		if !strings.HasPrefix(filepath.Clean(dstPath), cleanBase) {
			fmt.Fprintf(os.Stderr, "    %s[WARN]%s path escapes output dir, skipped: %s\n", colorYellow, colorReset, entry.Path)
			escaped++
			continue
		}
		jobs = append(jobs, fileJob{
			entry:   entry,
			srcPath: filepath.Join(packDir, entry.Path),
			dstPath: dstPath,
		})
	}

	results := mapConcurrent(jobs, func(job fileJob) fileResult {
		wasDecrypted, err := processFile(job.entry, job.srcPath, job.dstPath)
		return fileResult{path: job.entry.Path, decrypted: wasDecrypted, err: err}
	})

	stats := packStats{errors: escaped}
	for _, r := range results {
		switch {
		case r.err != nil:
			fmt.Fprintf(os.Stderr, "    %s[ERR]%s %s: %v\n", colorRed, colorReset, r.path, r.err)
			stats.errors++
		case r.decrypted:
			stats.decrypted++
		default:
			stats.copied++
		}
	}
	return stats
}

func processFile(entry contentsEntry, srcPath, dstPath string) (decrypted bool, err error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return false, err
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return false, err
	}
	if info.IsDir() {
		// contents.json may list bare directory paths as marker entries
		// (path with empty key, no file body). Mirror the directory and move on.
		return false, os.MkdirAll(dstPath, 0755)
	}

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return false, err
	}

	// manifest.json stays plaintext - the client parses it before it has
	// the content key - so copy it as-is even if an entry lists a key.
	if entry.Key == "" || entry.Path == manifestJSON {
		return false, os.WriteFile(dstPath, raw, 0644)
	}

	dec, err := cfb8.Decrypt(raw, []byte(entry.Key))
	if err != nil {
		return false, fmt.Errorf("decrypt %s: %w", entry.Path, err)
	}
	return true, os.WriteFile(dstPath, dec, 0644)
}

// copyIfMissing copies a plain pack file (manifest / icon) from packDir to
// outDir when it exists there but wasn't already written from contents.json.
func copyIfMissing(packDir, outDir, name string) error {
	src := filepath.Join(packDir, name)
	dst := filepath.Join(outDir, name)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, in)
	return err
}
