package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandertv/gophertunnel/minecraft/resource"
)

// zipDir creates a zip archive at dstPath from all files in srcDir.
func zipDir(srcDir, dstPath string) (retErr error) {
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); retErr == nil {
			retErr = cerr
		}
	}()

	w := zip.NewWriter(f)
	defer func() {
		if cerr := w.Close(); retErr == nil {
			retErr = cerr
		}
	}()

	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(srcDir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fw, err := w.Create(rel)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	})
}

// extractResourcePack reads a gophertunnel resource.Pack into a zip archive
// and extracts it to outDir.
func extractResourcePack(pack *resource.Pack, outDir string) (int, error) {
	size := pack.Len()
	buf := make([]byte, size)
	if _, err := pack.ReadAt(buf, 0); err != nil && err != io.EOF {
		return 0, fmt.Errorf("read pack data: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf), int64(size))
	if err != nil {
		return 0, fmt.Errorf("open zip: %w", err)
	}
	return extractZip(zr, outDir)
}

// extractZip writes all files from a zip.Reader into outDir, creating
// subdirectories as needed. Returns the number of files written.
func extractZip(zr *zip.Reader, outDir string) (int, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, err
	}

	cleanBase := filepath.Clean(outDir) + string(os.PathSeparator)
	count := 0
	for _, f := range zr.File {
		fpath := filepath.Join(outDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(fpath), cleanBase) {
			continue
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(fpath, 0755)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
			return count, err
		}

		if err := extractZipFile(f, fpath); err != nil {
			return count, fmt.Errorf("%s: %w", f.Name, err)
		}
		count++
	}
	return count, nil
}

func extractZipFile(f *zip.File, dst string) (err error) {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()

	_, err = io.Copy(out, rc)
	return err
}
