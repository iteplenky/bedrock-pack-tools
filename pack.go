package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/iteplenky/bedrock-pack-tools/v3/internal/lang"
	"github.com/iteplenky/gophertunnel/minecraft/protocol"
	"github.com/iteplenky/gophertunnel/minecraft/protocol/packet"
)

const (
	contentsJSON       = "contents.json"
	manifestJSON       = "manifest.json"
	packIconPNG        = "pack_icon.png"
	keysSuffix         = "_keys.json"
	keysFileName       = "keys.json"
	mcpackExt          = ".mcpack"
	contentsHeaderSize = 256
)

var contentsHeaderMagic = [4]byte{0xFC, 0xB9, 0xCF, 0x9B}

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

// keyStore is a thread-safe key collection that persists to JSON.
type keyStore struct {
	mu   sync.Mutex
	keys map[string]keyEntry
	file string
}

func newKeyStore(file string) *keyStore {
	return &keyStore{
		keys: make(map[string]keyEntry),
		file: file,
	}
}

// merge adds keys and persists. Persist happens under the mutex so
// concurrent merges can't overwrite each other's snapshot.
func (ks *keyStore) merge(collected map[string]keyEntry) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	maps.Copy(ks.keys, collected)
	if len(ks.keys) == 0 {
		return
	}
	if err := saveKeys(ks.keys, ks.file); err != nil {
		fmt.Fprintf(os.Stderr, lang.T("packs.warn.keysSaveFailed"), err)
	}
}

func (ks *keyStore) snapshot() map[string]keyEntry {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return maps.Clone(ks.keys)
}

func (ks *keyStore) count() int {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return len(ks.keys)
}

// parseResourcePacks decodes a ResourcePacksInfo payload. recover()
// catches protocol.Reader panics on malformed data and returns them
// as errPackBadProtocol.
func parseResourcePacks(payload []byte) (packs []protocol.TexturePackInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: payload: %v", errPackBadProtocol, r)
		}
	}()
	pk := &packet.ResourcePacksInfo{}
	r := protocol.NewReader(bytes.NewBuffer(payload), 0, false)
	pk.Marshal(r)
	return pk.TexturePacks, nil
}

// sanitizeServerAddr maps non-alphanumeric to underscore for use in
// filenames (server:port -> safe file stem).
func sanitizeServerAddr(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

// sanitizePackName preserves Unicode letters/digits so non-Latin pack
// names ("Кириллический Pack", "日本語パック") survive. Spaces become
// underscores; dashes and underscores are kept; everything else is dropped.
func sanitizePackName(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == ' ':
			return '_'
		case r == '_' || r == '-':
			return r
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return r
		}
		return -1
	}, name)
}

// collectKeys extracts encryption keys from ResourcePacksInfo.
// Name field is set to UUID because TexturePackInfo has no pack name -
// the real name is only available post-download via resource.Pack.
func collectKeys(packs []protocol.TexturePackInfo) map[string]keyEntry {
	keys := make(map[string]keyEntry)
	for _, tp := range packs {
		if tp.ContentKey == "" {
			continue
		}
		keys[tp.UUID.String()] = keyEntry{
			Key:     tp.ContentKey,
			Version: tp.Version,
			Name:    tp.UUID.String(),
		}
	}
	return keys
}

func readPackUUID(packDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(packDir, manifestJSON))
	if err != nil {
		return "", fmt.Errorf("%w: read: %w", errPackBadManifest, err)
	}
	var manifest struct {
		Header struct {
			UUID string `json:"uuid"`
		} `json:"header"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("%w: parse: %w", errPackBadManifest, err)
	}
	if manifest.Header.UUID == "" {
		return "", fmt.Errorf("%w: no header.uuid", errPackNoManifest)
	}
	return manifest.Header.UUID, nil
}

// readPackName returns the pack name from manifest.json, or "".
// Used to rename CDN-downloaded packs from UUID to human-readable.
func readPackName(packDir string) string {
	data, err := os.ReadFile(filepath.Join(packDir, manifestJSON))
	if err != nil {
		return ""
	}
	var manifest struct {
		Header struct {
			Name string `json:"name"`
		} `json:"header"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}
	return manifest.Header.Name
}

func saveKeys(keys map[string]keyEntry, path string) error {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write so the secret keys never persist half-written, and so an
	// existing looser-mode file gets retightened (os.WriteFile would not).
	return atomicWriteFile(path, "._keys-*.tmp", data, 0600)
}
