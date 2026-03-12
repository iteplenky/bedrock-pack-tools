package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

type keyEntry struct {
	Key     string `json:"key"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

func decryptContentsJSON(data []byte, packKey string) (*contentsFile, error) {
	if len(data) < 0x100 {
		return nil, fmt.Errorf("contents.json too small (%d bytes)", len(data))
	}
	encrypted := data[0x100:]
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

func runDecrypt(args []string) {
	if len(args) < 1 {
		fmt.Println(`Usage:
  bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]
  bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]

Decrypt a single encrypted resource pack:
  bedrock-pack-tools decrypt ./my_packs/SomePack_v1.0.0 YOUR_32_CHAR_KEY

Batch-decrypt all packs matched by a keys.json file:
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/
  bedrock-pack-tools decrypt --all my_keys.json ./my_packs/ ./decrypted/`)
		os.Exit(1)
	}

	if args[0] == "--all" {
		if len(args) < 3 {
			fmt.Println("Usage: bedrock-pack-tools decrypt --all <keys.json> <packs-dir> [output-dir]")
			os.Exit(1)
		}
		outDir := ""
		if len(args) > 3 {
			outDir = args[3]
		}
		decryptAll(args[1], args[2], outDir)
	} else {
		if len(args) < 2 {
			fmt.Println("Usage: bedrock-pack-tools decrypt <pack-dir> <key> [output-dir]")
			os.Exit(1)
		}
		packDir := args[0]
		packKey := args[1]
		outDir := packDir + "_decrypted"
		if len(args) > 2 {
			outDir = args[2]
		}
		decryptPack(packDir, packKey, outDir)
	}
}

type packJob struct {
	name   string
	dir    string
	key    string
	outDir string
}

func decryptAll(keysFile, cacheDir, outBase string) {
	data, err := os.ReadFile(keysFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", keysFile, err)
		os.Exit(1)
	}
	var keys map[string]keyEntry
	if err := json.Unmarshal(data, &keys); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", keysFile, err)
		os.Exit(1)
	}

	if outBase == "" {
		outBase = filepath.Join(cacheDir, "decrypted")
	}

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", cacheDir, err)
		os.Exit(1)
	}

	var jobs []packJob
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		packDir := filepath.Join(cacheDir, entry.Name())
		contentsPath := filepath.Join(packDir, "contents.json")

		if _, err := os.Stat(contentsPath); err != nil {
			continue
		}

		manifestPath := filepath.Join(packDir, "manifest.json")
		mdata, err := os.ReadFile(manifestPath)
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

		uid := manifest.Header.UUID
		keyInfo, ok := keys[uid]
		if !ok {
			fmt.Printf("  \033[33m[SKIP]\033[0m %s — no key for UUID %s\n", entry.Name(), uid)
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
		return
	}

	workers := runtime.NumCPU()
	if workers > len(jobs) {
		workers = len(jobs)
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		matched int
		jobCh   = make(chan packJob, len(jobs))
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
					fmt.Printf("  \033[31m[ERROR]\033[0m %s: %v\n", job.name, err)
				} else {
					fmt.Printf("  \033[36m[OK]\033[0m %s (%d decrypted, %d copied, %d errors)\n",
						job.name, stats.decrypted, stats.copied, stats.errors)
					matched++
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	fmt.Printf("\n  Decrypted %d/%d packs -> %s\n", matched, len(jobs), outBase)
}

func decryptPack(packDir, packKey, outDir string) {
	fmt.Println()
	fmt.Println("  Pack:   " + packDir)
	fmt.Println("  Key:    " + packKey)
	fmt.Println("  Output: " + outDir)
	fmt.Println()

	stats, err := decryptPackInner(packDir, packKey, outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  \033[31mError: %v\033[0m\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Done! %d decrypted, %d copied, %d errors\n",
		stats.decrypted, stats.copied, stats.errors)
}

type packStats struct {
	decrypted int32
	copied    int32
	errors    int32
}

func decryptPackInner(packDir, packKey, outDir string) (packStats, error) {
	contentsData, err := os.ReadFile(filepath.Join(packDir, "contents.json"))
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

	type fileJob struct {
		entry   contentsEntry
		srcPath string
		dstPath string
	}

	var jobs []fileJob
	for _, entry := range contents.Content {
		jobs = append(jobs, fileJob{
			entry:   entry,
			srcPath: filepath.Join(packDir, entry.Path),
			dstPath: filepath.Join(outDir, entry.Path),
		})
	}

	var (
		decrypted atomic.Int32
		copied    atomic.Int32
		errCount  atomic.Int32
		wg        sync.WaitGroup
		mu        sync.Mutex
		sem       = make(chan struct{}, runtime.NumCPU())
	)

	for _, job := range jobs {
		wg.Add(1)
		go func(j fileJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := processFile(j.entry, j.srcPath, j.dstPath, contents); err != nil {
				mu.Lock()
				fmt.Fprintf(os.Stderr, "    \033[31m[ERR]\033[0m %s: %v\n", j.entry.Path, err)
				mu.Unlock()
				errCount.Add(1)
				return
			}

			switch {
			case j.entry.Path == "contents.json" || j.entry.Path == "manifest.json" || j.entry.Key == "":
				copied.Add(1)
			default:
				decrypted.Add(1)
			}
		}(job)
	}

	wg.Wait()

	if err := copyPackIcon(packDir, outDir); err != nil {
		errCount.Add(1)
	}

	return packStats{
		decrypted: decrypted.Load(),
		copied:    copied.Load(),
		errors:    errCount.Load(),
	}, nil
}

func processFile(entry contentsEntry, srcPath, dstPath string, contents *contentsFile) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	if entry.Path == "contents.json" {
		cj, err := json.MarshalIndent(contents, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, cj, 0644)
	}

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	if entry.Key == "" || entry.Path == "manifest.json" {
		return os.WriteFile(dstPath, raw, 0644)
	}

	dec, err := decryptAES256CFB8(raw, []byte(entry.Key))
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", entry.Path, err)
	}
	return os.WriteFile(dstPath, dec, 0644)
}

func copyPackIcon(packDir, outDir string) error {
	iconSrc := filepath.Join(packDir, "pack_icon.png")
	iconDst := filepath.Join(outDir, "pack_icon.png")
	if _, err := os.Stat(iconSrc); err != nil {
		return nil
	}
	if _, err := os.Stat(iconDst); err == nil {
		return nil
	}
	raw, err := os.ReadFile(iconSrc)
	if err != nil {
		return err
	}
	return os.WriteFile(iconDst, raw, 0644)
}
