package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

func runEncrypt(args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage:
  bedrock-pack-tools encrypt <pack-dir> [key] [output.mcpack]

Encrypt a plain resource pack directory using AES-256-CFB8.
Produces a ready-to-use .mcpack file and a .mcpack.key file.

If the key is omitted, a random 32-character key is generated.
If the output is omitted, it defaults to <pack-name>.mcpack in the current directory.

Examples:
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567
  bedrock-pack-tools encrypt ./MyPack_v1.0.0/ MY_32_CHARACTER_KEY_HERE_1234567 ./out/MyPack.mcpack`)
		return errUsage
	}

	packDir := args[0]
	if _, err := os.Stat(filepath.Join(packDir, manifestJSON)); err != nil {
		return fmt.Errorf("%s does not contain %s", packDir, manifestJSON)
	}

	masterKey := ""
	if len(args) >= 2 {
		masterKey = args[1]
		if len(masterKey) != 32 {
			return fmt.Errorf("key must be exactly 32 characters, got %d", len(masterKey))
		}
	} else {
		var err error
		masterKey, err = generateKey()
		if err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
	}

	mcpackPath := filepath.Base(strings.TrimRight(packDir, "/\\")) + mcpackExt
	if len(args) >= 3 {
		mcpackPath = args[2]
	}
	if !strings.HasSuffix(mcpackPath, mcpackExt) {
		mcpackPath += mcpackExt
	}
	keyPath := mcpackPath + ".key"

	fmt.Println()
	fmt.Println("  Pack:    " + packDir)
	fmt.Println("  Key:     " + masterKey)
	fmt.Println("  Output:  " + mcpackPath)
	fmt.Println("  Keyfile: " + keyPath)
	fmt.Println()

	tmpDir, err := os.MkdirTemp("", "bedrock-encrypt-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	stats, err := encryptPack(packDir, masterKey, tmpDir)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(mcpackPath), 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := zipDir(tmpDir, mcpackPath); err != nil {
		return fmt.Errorf("create mcpack: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(masterKey), 0600); err != nil {
		return fmt.Errorf("write key file: %w", err)
	}

	fmt.Printf("  Done! %d encrypted, %d copied, %d errors\n",
		stats.encrypted, stats.copied, stats.errors)
	fmt.Printf("  %s%s%s (%s)\n", colorGreen, mcpackPath, colorReset, humanSize(fileSize(mcpackPath)))
	fmt.Printf("  %s%s%s\n", colorGreen, keyPath, colorReset)
	return nil
}

type encryptStats struct {
	encrypted int
	copied    int
	errors    int
}

func encryptPack(packDir, masterKey, outDir string) (encryptStats, error) {
	if len(masterKey) != 32 {
		return encryptStats{}, fmt.Errorf("master key must be exactly 32 characters, got %d", len(masterKey))
	}

	packUUID, err := readPackUUID(packDir)
	if err != nil {
		return encryptStats{}, err
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return encryptStats{}, fmt.Errorf("create output dir: %w", err)
	}

	var files []string
	err = filepath.WalkDir(packDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(packDir, path)
		if relErr != nil {
			return relErr
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return encryptStats{}, fmt.Errorf("walk pack dir: %w", err)
	}

	type result struct {
		entry contentsEntry
		err   error
	}

	type fileJob struct {
		relPath string
		srcPath string
		dstPath string
	}

	jobCh := make(chan fileJob, runtime.NumCPU())
	resultCh := make(chan result, len(files))

	workers := min(runtime.NumCPU(), len(files))
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for job := range jobCh {
				entry, err := encryptFile(job.relPath, job.srcPath, job.dstPath)
				resultCh <- result{entry: entry, err: err}
			}
		}()
	}

	for _, rel := range files {
		if rel == contentsJSON {
			continue
		}
		jobCh <- fileJob{
			relPath: rel,
			srcPath: filepath.Join(packDir, rel),
			dstPath: filepath.Join(outDir, rel),
		}
	}
	close(jobCh)

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var (
		stats   encryptStats
		entries []contentsEntry
	)
	for r := range resultCh {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "    %s[ERR]%s %s: %v\n", colorRed, colorReset, r.entry.Path, r.err)
			stats.errors++
			continue
		}
		if r.entry.Key != "" {
			stats.encrypted++
		} else {
			stats.copied++
		}
		entries = append(entries, r.entry)
	}

	slices.SortFunc(entries, func(a, b contentsEntry) int {
		return strings.Compare(a.Path, b.Path)
	})

	contentsData, err := buildEncryptedContents(packUUID, masterKey, entries)
	if err != nil {
		return encryptStats{}, fmt.Errorf("build contents.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, contentsJSON), contentsData, 0644); err != nil {
		return encryptStats{}, fmt.Errorf("write contents.json: %w", err)
	}

	return stats, nil
}

func encryptFile(relPath, srcPath, dstPath string) (contentsEntry, error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return contentsEntry{Path: relPath}, err
	}

	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return contentsEntry{Path: relPath}, err
	}

	if relPath == manifestJSON || relPath == packIconPNG {
		return contentsEntry{Path: relPath, Key: ""}, os.WriteFile(dstPath, raw, 0644)
	}

	fileKey, err := generateKey()
	if err != nil {
		return contentsEntry{Path: relPath}, fmt.Errorf("generate key: %w", err)
	}

	enc, err := encryptAES256CFB8(raw, []byte(fileKey))
	if err != nil {
		return contentsEntry{Path: relPath}, fmt.Errorf("encrypt: %w", err)
	}

	return contentsEntry{Path: relPath, Key: fileKey}, os.WriteFile(dstPath, enc, 0644)
}

func buildEncryptedContents(packUUID, masterKey string, entries []contentsEntry) ([]byte, error) {
	cf := contentsFile{Content: entries}
	payload, err := json.Marshal(cf)
	if err != nil {
		return nil, err
	}

	encrypted, err := encryptAES256CFB8(payload, []byte(masterKey))
	if err != nil {
		return nil, err
	}

	header := buildContentsHeader(packUUID)
	return append(header, encrypted...), nil
}

// buildContentsHeader creates the 256-byte binary header for contents.json.
// Layout: [version:4][magic:4][padding:8][0x24:1][uuid-ascii:36][zeros:203]
func buildContentsHeader(packUUID string) []byte {
	header := make([]byte, contentsHeaderSize)
	// bytes 0-3: version 0 (LE uint32, already zero)
	copy(header[4:8], contentsHeaderMagic[:])
	// bytes 8-15: padding (zeros)
	header[16] = 0x24 // '$' separator before UUID
	copy(header[17:], []byte(packUUID))
	return header
}

const keyAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateKey() (string, error) {
	alphabetLen := big.NewInt(int64(len(keyAlphabet)))
	b := make([]byte, 32)
	for i := range b {
		n, err := rand.Int(rand.Reader, alphabetLen)
		if err != nil {
			return "", err
		}
		b[i] = keyAlphabet[n.Int64()]
	}
	return string(b), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / float64(unit)
	if kb < unit {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / float64(unit)
	if mb < unit {
		return fmt.Sprintf("%.1f MB", mb)
	}
	return fmt.Sprintf("%.1f GB", mb/float64(unit))
}
