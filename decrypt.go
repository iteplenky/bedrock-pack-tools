package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

type contentsEntry struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

type contentsFile struct {
	Content []contentsEntry `json:"content"`
}

// contentsHeaderSize is the binary header (magic + pack UUID) prepended
// to the encrypted JSON in contents.json.
const contentsHeaderSize = 256

func decryptContentsJSON(data []byte, packKey string) (*contentsFile, error) {
	if len(data) < contentsHeaderSize {
		return nil, fmt.Errorf("contents.json too small (%d bytes)", len(data))
	}
	encrypted := data[contentsHeaderSize:]
	plaintext, err := decryptAES256CFB8(encrypted, []byte(packKey))
	if err != nil {
		return nil, fmt.Errorf("decrypt contents.json: %w", err)
	}
	plaintext = bytes.TrimRight(plaintext, "\x00 \n\r\t")

	var contents contentsFile
	if err := json.Unmarshal(plaintext, &contents); err != nil {
		preview := string(plaintext)
		if len(preview) > 100 {
			preview = preview[:100]
		}
		return nil, fmt.Errorf("parse contents.json: %w (first 100 bytes: %q)", err, preview)
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
	outDir := args[0] + "_decrypted"
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

func decryptAll(keysFile, cacheDir, outBase string) error {
	data, err := os.ReadFile(keysFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", keysFile, err)
	}
	var keys map[string]keyEntry
	if err := json.Unmarshal(data, &keys); err != nil {
		return fmt.Errorf("parse %s: %w", keysFile, err)
	}

	if outBase == "" {
		outBase = filepath.Join(cacheDir, "decrypted")
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", cacheDir, err)
	}

	var jobs []packJob
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		packDir := filepath.Join(cacheDir, entry.Name())

		if _, err := os.Stat(filepath.Join(packDir, contentsJSON)); err != nil {
			continue
		}

		mdata, err := os.ReadFile(filepath.Join(packDir, manifestJSON))
		if err != nil {
			continue
		}
		var manifest struct {
			Header struct {
				UUID string `json:"uuid"`
			} `json:"header"`
		}
		if err := json.Unmarshal(mdata, &manifest); err != nil {
			continue
		}

		keyInfo, ok := keys[manifest.Header.UUID]
		if !ok {
			fmt.Printf("  %s[SKIP]%s %s — no key for UUID %s\n",
				colorYellow, colorReset, entry.Name(), manifest.Header.UUID)
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
	fmt.Printf("\n  Decrypted %d/%d packs -> %s\n", succeeded, len(jobs), outBase)
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
	decrypted int32
	copied    int32
	errors    int32
}

func decryptPackInner(packDir, packKey, outDir string) (packStats, error) {
	contentsData, err := os.ReadFile(filepath.Join(packDir, contentsJSON))
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

	var (
		decrypted atomic.Int32
		copied    atomic.Int32
		errCount  atomic.Int32
	)

	cj, err := json.MarshalIndent(contents, "", "  ")
	if err != nil {
		return packStats{}, fmt.Errorf("marshal contents.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, contentsJSON), cj, 0644); err != nil {
		return packStats{}, fmt.Errorf("write contents.json: %w", err)
	}
	copied.Add(1)

	type fileJob struct {
		entry   contentsEntry
		srcPath string
		dstPath string
	}

	jobCh := make(chan fileJob, runtime.NumCPU())

	var wg sync.WaitGroup
	for range min(runtime.NumCPU(), len(contents.Content)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				wasDecrypted, err := processFile(job.entry, job.srcPath, job.dstPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "    %s[ERR]%s %s: %v\n", colorRed, colorReset, job.entry.Path, err)
					errCount.Add(1)
					continue
				}
				if wasDecrypted {
					decrypted.Add(1)
				} else {
					copied.Add(1)
				}
			}
		}()
	}

	for _, entry := range contents.Content {
		if entry.Path == contentsJSON {
			continue
		}
		jobCh <- fileJob{
			entry:   entry,
			srcPath: filepath.Join(packDir, entry.Path),
			dstPath: filepath.Join(outDir, entry.Path),
		}
	}
	close(jobCh)
	wg.Wait()

	if err := copyPackIcon(packDir, outDir); err != nil {
		fmt.Fprintf(os.Stderr, "    %s[ERR]%s %s: %v\n", colorRed, colorReset, packIconPNG, err)
		errCount.Add(1)
	}

	return packStats{
		decrypted: decrypted.Load(),
		copied:    copied.Load(),
		errors:    errCount.Load(),
	}, nil
}

func processFile(entry contentsEntry, srcPath, dstPath string) (decrypted bool, err error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return false, err
	}

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return false, err
	}

	if entry.Key == "" || entry.Path == manifestJSON {
		return false, os.WriteFile(dstPath, raw, 0644)
	}

	dec, err := decryptAES256CFB8(raw, []byte(entry.Key))
	if err != nil {
		return false, fmt.Errorf("decrypt %s: %w", entry.Path, err)
	}
	return true, os.WriteFile(dstPath, dec, 0644)
}

func copyPackIcon(packDir, outDir string) error {
	iconSrc := filepath.Join(packDir, packIconPNG)
	iconDst := filepath.Join(outDir, packIconPNG)
	if _, err := os.Stat(iconSrc); err != nil {
		return nil
	}
	if _, err := os.Stat(iconDst); err == nil {
		return nil
	}
	return copyFile(iconSrc, iconDst)
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
