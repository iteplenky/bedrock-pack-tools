package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildZip assembles an in-memory zip from a slice of (path, body) pairs
// and returns a *zip.Reader pointing at it. Mirrors what extractZip will
// see in production via extractResourcePack.
func buildZip(t *testing.T, entries []zipEntry) *zip.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		fw, err := zw.Create(e.name)
		if err != nil {
			t.Fatalf("zw.Create(%q): %v", e.name, err)
		}
		if _, err := fw.Write([]byte(e.body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zw.Close: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	return zr
}

type zipEntry struct {
	name string
	body string
}

// TestExtractZipBlocksZipSlip verifies the zip-slip defense at
// zip.go:90 actually prevents path traversal outside outDir. This is
// security-sensitive code with no other test coverage - a regression
// here would let a malicious .mcpack write to arbitrary filesystem
// locations.
func TestExtractZipBlocksZipSlip(t *testing.T) {
	outDir := t.TempDir()
	sentinel := filepath.Join(outDir, "..", "ESCAPED_SENTINEL")

	// Best-effort cleanup of the sentinel name regardless of test outcome
	// in case a regression actually wrote it. t.TempDir's automatic
	// cleanup does NOT cover paths outside the temp dir.
	t.Cleanup(func() { _ = os.Remove(sentinel) })

	zr := buildZip(t, []zipEntry{
		{name: "../ESCAPED_SENTINEL", body: "malicious"},
		{name: "../../etc/passwd", body: "more malicious"},
		{name: "subdir/../../also_escaped", body: "even more"},
		{name: "ok_file.txt", body: "benign"},
	})

	n, err := extractZip(zr, outDir)
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if n != 1 {
		t.Errorf("extracted %d files, want 1 (only ok_file.txt)", n)
	}

	if _, statErr := os.Stat(sentinel); statErr == nil {
		t.Fatalf("zip-slip succeeded: %s was written outside outDir", sentinel)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "ok_file.txt")); statErr != nil {
		t.Errorf("benign file was skipped: %v", statErr)
	}

	// Walk outDir and confirm nothing landed outside it.
	walkErr := filepath.WalkDir(outDir, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasPrefix(path, outDir) {
			t.Errorf("file outside outDir: %s", path)
		}
		return nil
	})
	if walkErr != nil {
		t.Errorf("walk outDir: %v", walkErr)
	}
}

// TestExtractZipNestedDirs covers the happy path with nested directory
// creation, which the security test bypasses by using flat paths.
func TestExtractZipNestedDirs(t *testing.T) {
	outDir := t.TempDir()
	zr := buildZip(t, []zipEntry{
		{name: "a/b/c/deep.txt", body: "nested"},
		{name: "a/sibling.txt", body: "sibling"},
	})
	n, err := extractZip(zr, outDir)
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if n != 2 {
		t.Errorf("extracted %d files, want 2", n)
	}
	for _, p := range []string{"a/b/c/deep.txt", "a/sibling.txt"} {
		if _, statErr := os.Stat(filepath.Join(outDir, p)); statErr != nil {
			t.Errorf("missing: %s (%v)", p, statErr)
		}
	}
}

// TestExtractZipEmpty checks an empty archive doesn't error and creates
// the output dir. extractZip's MkdirAll-up-front behaviour matters for
// the CDN download path where the caller assumes outDir exists.
func TestExtractZipEmpty(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "fresh")
	zr := buildZip(t, nil)
	n, err := extractZip(zr, outDir)
	if err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	if n != 0 {
		t.Errorf("extracted %d files, want 0", n)
	}
	if _, statErr := os.Stat(outDir); statErr != nil {
		t.Errorf("outDir not created: %v", statErr)
	}
}

// TestZipDirRoundtrip writes a few files via zipDir then reads them
// back via extractZip and confirms content matches. Catches encoding
// regressions in zipDir's WalkDir traversal.
func TestZipDirRoundtrip(t *testing.T) {
	src := t.TempDir()
	files := map[string]string{
		"manifest.json":    `{"header":{"uuid":"x"}}`,
		"sub/a.txt":        "hello",
		"sub/nested/b.bin": string([]byte{0, 1, 2, 3, 0xFF}),
	}
	for rel, body := range files {
		full := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	zipPath := filepath.Join(t.TempDir(), "out.zip")
	if err := zipDir(src, zipPath); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	zipData, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}

	outDir := t.TempDir()
	if _, err := extractZip(zr, outDir); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(outDir, rel))
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%s mismatch: got %q want %q", rel, got, want)
		}
	}
}
