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

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const (
	contentsJSON       = "contents.json"
	manifestJSON       = "manifest.json"
	packIconPNG        = "pack_icon.png"
	keysSuffix         = "_keys.json"
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

// keyStore manages a thread-safe collection of encryption keys
// with automatic persistence to a JSON file.
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

// merge adds collected keys and persists the updated set to disk.
func (ks *keyStore) merge(collected map[string]keyEntry) {
	ks.mu.Lock()
	for uid, entry := range collected {
		ks.keys[uid] = entry
	}
	snapshot := maps.Clone(ks.keys)
	ks.mu.Unlock()

	if len(snapshot) > 0 {
		if err := saveKeys(snapshot, ks.file); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not save keys: %v\n", err)
		}
	}
}

// snapshot returns a copy of all stored keys.
func (ks *keyStore) snapshot() map[string]keyEntry {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return maps.Clone(ks.keys)
}

// count returns the number of stored keys.
func (ks *keyStore) count() int {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return len(ks.keys)
}

// parseResourcePacks decodes a raw ResourcePacksInfo payload
// and returns the texture pack list. Recovers from panics caused
// by malformed data in the protocol reader.
func parseResourcePacks(payload []byte) (packs []protocol.TexturePackInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed ResourcePacksInfo payload: %v", r)
		}
	}()
	pk := &packet.ResourcePacksInfo{}
	r := protocol.NewReader(bytes.NewBuffer(payload), 0, false)
	pk.Marshal(r)
	return pk.TexturePacks, nil
}

// sanitizeServerAddr replaces all non-alphanumeric characters with underscores.
// Used for server addresses → safe file names.
func sanitizeServerAddr(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '_'
	}, s)
}

// sanitizePackName keeps alphanumeric, dash, underscore; replaces spaces
// with underscores; drops everything else. Used for pack directory names.
func sanitizePackName(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}
		if r == ' ' {
			return '_'
		}
		return -1
	}, name)
}

// collectKeys extracts encryption keys from the raw ResourcePacksInfo packet.
// Name is set to UUID because TexturePackInfo doesn't carry a human-readable
// pack name — the real name is only available after full download via resource.Pack.
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
		return "", fmt.Errorf("read manifest.json: %w", err)
	}
	var manifest struct {
		Header struct {
			UUID string `json:"uuid"`
		} `json:"header"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("parse manifest.json: %w", err)
	}
	if manifest.Header.UUID == "" {
		return "", fmt.Errorf("manifest.json has no header.uuid")
	}
	return manifest.Header.UUID, nil
}

func saveKeys(keys map[string]keyEntry, path string) error {
	data, err := json.MarshalIndent(keys, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
