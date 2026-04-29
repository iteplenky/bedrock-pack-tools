package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecryptContentsJSON(t *testing.T) {
	contents := contentsFile{
		Content: []contentsEntry{
			{Path: "textures/block.png", Key: "KEYKEYKEYKEYKEYKEYKEYKEYKEYKEY32"},
			{Path: "manifest.json", Key: ""},
		},
	}
	payload, _ := json.Marshal(contents)

	ciphertext := mustEncrypt(t, payload, []byte(testMasterKey))

	header := make([]byte, contentsHeaderSize)
	data := append(header, ciphertext...)

	result, err := decryptContentsJSON(data, testMasterKey)
	if err != nil {
		t.Fatalf("decryptContentsJSON error: %v", err)
	}
	if len(result.Content) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Content))
	}
	if result.Content[0].Path != "textures/block.png" {
		t.Errorf("entry[0].Path = %q, want %q", result.Content[0].Path, "textures/block.png")
	}
	if result.Content[0].Key != "KEYKEYKEYKEYKEYKEYKEYKEYKEYKEY32" {
		t.Errorf("entry[0].Key = %q", result.Content[0].Key)
	}
}

func TestDecryptContentsJSON_TooSmall(t *testing.T) {
	_, err := decryptContentsJSON(make([]byte, 100), testMasterKey)
	if err == nil {
		t.Error("expected error for data smaller than 256 bytes")
	}
}

func TestDecryptContentsJSON_InvalidJSON(t *testing.T) {
	garbage := mustEncrypt(t, []byte("not json {{{"), []byte(testMasterKey))
	header := make([]byte, contentsHeaderSize)
	data := append(header, garbage...)

	_, err := decryptContentsJSON(data, testMasterKey)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecryptContentsJSON_TrailingNulls(t *testing.T) {
	contents := contentsFile{Content: []contentsEntry{{Path: "a.txt", Key: ""}}}
	payload, _ := json.Marshal(contents)
	payload = append(payload, 0, 0, 0, '\n', ' ')

	ciphertext := mustEncrypt(t, payload, []byte(testMasterKey))
	header := make([]byte, contentsHeaderSize)
	data := append(header, ciphertext...)

	result, err := decryptContentsJSON(data, testMasterKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result.Content))
	}
}

func TestProcessFile_CopyPlain(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(srcDir, 0755)

	os.WriteFile(filepath.Join(srcDir, "readme.txt"), []byte("hello world"), 0644)

	entry := contentsEntry{Path: "readme.txt", Key: ""}

	decrypted, err := processFile(entry, filepath.Join(srcDir, "readme.txt"), filepath.Join(dstDir, "readme.txt"))
	if err != nil {
		t.Fatalf("processFile error: %v", err)
	}
	if decrypted {
		t.Error("plain file should not be reported as decrypted")
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "readme.txt"))
	if string(got) != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestProcessFile_DecryptEncrypted(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(srcDir, 0755)

	fileKey := "ZYXWVUTSRQPONMLKJIHGFEDCBA654321"
	plaintext := []byte(`{"type":"texture","data":"abc"}`)
	ciphertext := mustEncrypt(t, plaintext, []byte(fileKey))
	os.WriteFile(filepath.Join(srcDir, "data.json"), ciphertext, 0644)

	entry := contentsEntry{Path: "data.json", Key: fileKey}

	decrypted, err := processFile(entry, filepath.Join(srcDir, "data.json"), filepath.Join(dstDir, "data.json"))
	if err != nil {
		t.Fatalf("processFile error: %v", err)
	}
	if !decrypted {
		t.Error("encrypted file should be reported as decrypted")
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "data.json"))
	if string(got) != string(plaintext) {
		t.Errorf("got %q, want %q", got, plaintext)
	}
}

func TestProcessFile_ManifestCopiedPlain(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(srcDir, 0755)

	manifest := `{"format_version":2,"header":{"uuid":"abc"}}`
	os.WriteFile(filepath.Join(srcDir, "manifest.json"), []byte(manifest), 0644)

	entry := contentsEntry{Path: "manifest.json", Key: "some-key-that-should-be-ignored!"}

	decrypted, err := processFile(entry, filepath.Join(srcDir, "manifest.json"), filepath.Join(dstDir, "manifest.json"))
	if err != nil {
		t.Fatalf("processFile error: %v", err)
	}
	if decrypted {
		t.Error("manifest.json should be copied plain, not reported as decrypted")
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "manifest.json"))
	if string(got) != manifest {
		t.Errorf("manifest.json should be copied verbatim, got %q", got)
	}
}

func TestProcessFile_DirectoryMarker(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(filepath.Join(srcDir, "items"), 0755)
	os.WriteFile(filepath.Join(srcDir, "items", "child.json"), []byte("{}"), 0644)

	entry := contentsEntry{Path: "items", Key: ""}

	decrypted, err := processFile(entry, filepath.Join(srcDir, "items"), filepath.Join(dstDir, "items"))
	if err != nil {
		t.Fatalf("processFile should treat directory marker as no-op, got error: %v", err)
	}
	if decrypted {
		t.Error("directory marker should not be reported as decrypted")
	}

	info, err := os.Stat(filepath.Join(dstDir, "items"))
	if err != nil {
		t.Fatalf("expected dst directory to be created: %v", err)
	}
	if !info.IsDir() {
		t.Error("dst path should be a directory")
	}
}

func TestCopyPackIcon(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(srcDir, 0755)
	os.MkdirAll(dstDir, 0755)

	os.WriteFile(filepath.Join(srcDir, "pack_icon.png"), []byte("PNG_DATA"), 0644)

	err := copyPackIcon(srcDir, dstDir)
	if err != nil {
		t.Fatalf("copyPackIcon error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dstDir, "pack_icon.png"))
	if string(got) != "PNG_DATA" {
		t.Errorf("got %q, want %q", got, "PNG_DATA")
	}
}

func TestCopyPackIcon_AlreadyExists(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	dstDir := filepath.Join(tmp, "dst")
	os.MkdirAll(srcDir, 0755)
	os.MkdirAll(dstDir, 0755)

	os.WriteFile(filepath.Join(srcDir, "pack_icon.png"), []byte("NEW"), 0644)
	os.WriteFile(filepath.Join(dstDir, "pack_icon.png"), []byte("OLD"), 0644)

	copyPackIcon(srcDir, dstDir)

	got, _ := os.ReadFile(filepath.Join(dstDir, "pack_icon.png"))
	if string(got) != "OLD" {
		t.Error("should not overwrite existing icon")
	}
}

func TestCopyPackIcon_NoIcon(t *testing.T) {
	tmp := t.TempDir()
	err := copyPackIcon(filepath.Join(tmp, "src"), filepath.Join(tmp, "dst"))
	if err != nil {
		t.Errorf("expected nil error when no icon exists, got %v", err)
	}
}
