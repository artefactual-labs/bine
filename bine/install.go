package bine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/mholt/archives"
)

func replaceFile(src, dst string) error {
	backupPath := dst + ".old"
	_ = os.Remove(backupPath)
	backupCreated := false

	if info, err := os.Stat(dst); err == nil {
		if info.IsDir() {
			return fmt.Errorf("destination %q is a directory", dst)
		}
		if err := os.Rename(dst, backupPath); err != nil {
			return err
		}
		backupCreated = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(src, dst); err != nil {
		if backupCreated {
			_ = os.Rename(backupPath, dst)
		}
		return err
	}

	if backupCreated {
		return os.Remove(backupPath)
	}

	return nil
}

// openDownload starts a download; the caller owns the response body.
func openDownload(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download asset from %q: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("download failed: status %s (%s)", resp.Status, url)
	}
	return resp.Body, nil
}

func installRecipe(ctx context.Context, client *http.Client, url, target string) error {
	body, err := openDownload(ctx, client, url)
	if err != nil {
		return err
	}
	defer body.Close()
	file, err := os.CreateTemp("", "downloaded-*-"+filepath.Base(url))
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := io.Copy(file, body); err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to reset file pointer: %w", err)
	}
	staged, err := os.CreateTemp(filepath.Dir(target), ".bine-bin-install-*")
	if err != nil {
		return fmt.Errorf("create temporary binary: %w", err)
	}
	stagedPath := staged.Name()
	_ = staged.Close()
	defer os.Remove(stagedPath)
	if err := extract(ctx, file, stagedPath); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	if err := replaceFile(stagedPath, target); err != nil {
		return fmt.Errorf("move installed binary: %w", err)
	}
	return nil
}

// extract the binary from the archive file and writes it to binPath.
func extract(ctx context.Context, osf *os.File, binPath string) error {
	fsys, err := archives.FileSystem(ctx, osf.Name(), osf)
	if err != nil {
		return fmt.Errorf("archives.FileSystem: %v", err)
	}

	f, err := findBinary(fsys, filepath.Base(binPath))
	if errors.Is(err, archives.NoMatch) {
		f = osf // TODO: archifes.FileFS is not reliable atm.
	} else if err != nil {
		return fmt.Errorf("find binary: %v", err)
	}

	// Create (or truncate) the destination file at binPath.
	dest, err := os.Create(binPath)
	if err != nil {
		return err
	}
	defer func() { _ = dest.Close() }()

	// Copy the contents of the extracted file to the destination.
	if _, err := io.Copy(dest, f); err != nil {
		return fmt.Errorf("copy: %v", err)
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}

	return nil
}

// findBinary searches the filesystem for a binary file with the given name.
//
// This is used when extracting a binary from an archive.
func findBinary(fsys fs.FS, name string) (_ fs.File, err error) {
	if _, ok := fsys.(archives.FileFS); ok {
		// TODO: open and return it once they fix the issue with brotli matching.
		return nil, archives.NoMatch
	}

	var match string
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == ".git" {
			return fs.SkipDir
		}
		if !d.IsDir() {
			// We have a match if the filename matches or the file is executable.
			if filepath.Base(path) == name {
				match = path
			} else {
				if f, err := fsys.Open(path); err == nil {
					if info, err := f.Stat(); err == nil {
						if perm := info.Mode().Perm(); perm&0o111 != 0 {
							match = path
						}
					}
					_ = f.Close()
				}
			}
		}
		if match != "" {
			return fs.SkipAll
		}
		return nil
	}); err != nil {
		return nil, err
	}

	if match == "" {
		return nil, fmt.Errorf("no match for %q", name)
	}

	f, err := fsys.Open(match)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// checksum computes the SHA256 checksum of the file at filePath.
func checksum(filePath string) (string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", err
	} else if err != nil {
		return "", err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	hash := h.Sum(nil)

	return hex.EncodeToString(hash), nil
}
