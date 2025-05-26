package bine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mholt/archives"
)

// Supporting functions for installing binaries.

// goInstall installs a Go tool using 'go install'.
//
// TODO: honour binPath - name the binary following the user's preference.
func goInstall(ctx context.Context, b *bin, binDir string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("cannot find 'go' command: %v", err)
	}

	version := b.canonicalVersion()
	if version == "" {
		version = "latest"
	}

	packageName := fmt.Sprintf("%s@%s", b.GoPackage, version)

	binDir, err = filepath.Abs(binDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for bin directory %s: %v", binDir, err)
	}

	cmd := execCommand(ctx, goBin, "install", packageName)

	// Set GOBIN to install the binary there. fakeExecCommand sets cmd.Env so
	// we can't assume it's empty.
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "GOBIN="+binDir)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = "(no stderr output)"
		}
		return fmt.Errorf("`go install %s` failed: %v\nstderr: %s", packageName, err, msg)
	}

	return nil
}

func binInstall(ctx context.Context, client *http.Client, b *bin, binPath string) error {
	downloadURL, err := b.provider.downloadURL(b)
	if err != nil {
		return fmt.Errorf("failed to generate download URL: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Download the asset.
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download asset from %q: %v", downloadURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %s (%s)", resp.Status, downloadURL)
	}
	f, err := os.CreateTemp("", "downloaded-*-"+filepath.Base(downloadURL))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Reset file pointer to the beginning so we can extract.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to reset file pointer: %v", err)
	}

	if err := extract(ctx, f, binPath); err != nil {
		return fmt.Errorf("extract failed: %v", err)
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
