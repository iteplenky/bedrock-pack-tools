package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestPack(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pack")
	os.MkdirAll(filepath.Join(dir, "textures"), 0755)

	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{
		"format_version": 2,
		"header": {
			"uuid": "01234567-89ab-cdef-0123-456789abcdef",
			"name": "Test Pack",
			"version": [1, 0, 0]
		}
	}`), 0644)

	os.WriteFile(filepath.Join(dir, "pack_icon.png"), []byte("FAKE_PNG_DATA"), 0644)
	os.WriteFile(filepath.Join(dir, "textures/terrain.png"), []byte("terrain texture data here"), 0644)
	os.WriteFile(filepath.Join(dir, "textures/items.json"), []byte(`{"resource_pack_name":"test"}`), 0644)
	os.WriteFile(filepath.Join(dir, "sounds.json"), []byte(`{"sounds":[]}`), 0644)

	return dir
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	packDir := setupTestPack(t)
	encDir := filepath.Join(t.TempDir(), "encrypted")
	decDir := filepath.Join(t.TempDir(), "decrypted")

	encStats, err := encryptPack(packDir, testMasterKey, encDir)
	if err != nil {
		t.Fatalf("encryptPack: %v", err)
	}
	if encStats.encrypted == 0 {
		t.Fatal("expected at least one encrypted file")
	}
	if encStats.errors != 0 {
		t.Fatalf("encrypt had %d errors", encStats.errors)
	}

	if _, err := os.Stat(filepath.Join(encDir, "contents.json")); err != nil {
		t.Fatal("contents.json not created")
	}

	decStats, err := decryptPackInner(encDir, testMasterKey, decDir)
	if err != nil {
		t.Fatalf("decryptPackInner: %v", err)
	}
	if decStats.errors != 0 {
		t.Fatalf("decrypt had %d errors", decStats.errors)
	}

	origFiles := map[string]string{
		"textures/terrain.png": "terrain texture data here",
		"textures/items.json":  `{"resource_pack_name":"test"}`,
		"sounds.json":          `{"sounds":[]}`,
	}
	for rel, want := range origFiles {
		got, err := os.ReadFile(filepath.Join(decDir, rel))
		if err != nil {
			t.Errorf("read decrypted %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", rel, got, want)
		}
	}

	manifest, err := os.ReadFile(filepath.Join(decDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read decrypted manifest.json: %v", err)
	}
	origManifest, _ := os.ReadFile(filepath.Join(packDir, "manifest.json"))
	if string(manifest) != string(origManifest) {
		t.Error("manifest.json was modified during encrypt->decrypt")
	}

	icon, err := os.ReadFile(filepath.Join(decDir, "pack_icon.png"))
	if err != nil {
		t.Fatalf("read decrypted pack_icon.png: %v", err)
	}
	if string(icon) != "FAKE_PNG_DATA" {
		t.Errorf("pack_icon.png: got %q, want %q", icon, "FAKE_PNG_DATA")
	}
}

func TestEncryptPack_GeneratedKey(t *testing.T) {
	packDir := setupTestPack(t)
	encDir := filepath.Join(t.TempDir(), "encrypted")

	key, err := generateKey()
	if err != nil {
		t.Fatalf("generateKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d, want 32", len(key))
	}

	_, err = encryptPack(packDir, key, encDir)
	if err != nil {
		t.Fatalf("encryptPack: %v", err)
	}

	decDir := filepath.Join(t.TempDir(), "decrypted")
	_, err = decryptPackInner(encDir, key, decDir)
	if err != nil {
		t.Fatalf("decryptPackInner with generated key: %v", err)
	}
}

func TestEncryptPack_NoManifest(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644)

	_, err := encryptPack(dir, testMasterKey, filepath.Join(t.TempDir(), "out"))
	if err == nil {
		t.Error("expected error for pack without manifest.json")
	}
}

func TestBuildContentsHeader(t *testing.T) {
	uuid := "01234567-89ab-cdef-0123-456789abcdef"
	header := buildContentsHeader(uuid)

	if len(header) != contentsHeaderSize {
		t.Fatalf("header size = %d, want %d", len(header), contentsHeaderSize)
	}
	if header[4] != 0xFC || header[5] != 0xB9 || header[6] != 0xCF || header[7] != 0x9B {
		t.Errorf("magic bytes: %x, want fcb9cf9b", header[4:8])
	}
	if header[16] != 0x24 {
		t.Errorf("separator byte: 0x%02x, want 0x24 ('$')", header[16])
	}
	if string(header[17:17+len(uuid)]) != uuid {
		t.Errorf("UUID in header: %q, want %q", header[17:17+len(uuid)], uuid)
	}
}

func TestZipDirAndKeyFile(t *testing.T) {
	packDir := setupTestPack(t)
	mcpackPath := filepath.Join(t.TempDir(), "Test.mcpack")
	keyPath := mcpackPath + ".key"

	tmpEnc := filepath.Join(t.TempDir(), "enc")
	_, err := encryptPack(packDir, testMasterKey, tmpEnc)
	if err != nil {
		t.Fatalf("encryptPack: %v", err)
	}

	if err := zipDir(tmpEnc, mcpackPath); err != nil {
		t.Fatalf("zipDir: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte(testMasterKey), 0644); err != nil {
		t.Fatalf("write key: %v", err)
	}

	info, err := os.Stat(mcpackPath)
	if err != nil {
		t.Fatalf("mcpack not created: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("mcpack is empty")
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("key file not created: %v", err)
	}
	if string(keyData) != testMasterKey {
		t.Errorf("key file: got %q, want %q", keyData, testMasterKey)
	}
}

func TestGenerateKey(t *testing.T) {
	seen := make(map[string]bool)
	for range 100 {
		key, err := generateKey()
		if err != nil {
			t.Fatalf("generateKey: %v", err)
		}
		if len(key) != 32 {
			t.Fatalf("key length = %d, want 32", len(key))
		}
		for _, c := range key {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				t.Fatalf("invalid character in key: %c", c)
			}
		}
		if seen[key] {
			t.Fatalf("duplicate key generated: %s", key)
		}
		seen[key] = true
	}
}
