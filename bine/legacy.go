package bine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
)

func cached(binPath, versionMarker string) bool {
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return false
	}

	if _, err := os.Stat(versionMarker); os.IsNotExist(err) {
		return false
	}

	return true
}

func installed(b *bin, cacheDir string) bool {
	binDir := filepath.Join(cacheDir, "bin")
	versionsDir := filepath.Join(cacheDir, "versions", b.Name)
	binPath := filepath.Join(binDir, b.Name)
	versionMarker := filepath.Join(versionsDir, b.Version)

	return cached(binPath, versionMarker)
}

func ensureInstalled(ctx context.Context, client *http.Client, b *bin, cacheDir string) (ret string, err error) {
	binDir := filepath.Join(cacheDir, "bin")
	versionsDir := filepath.Join(cacheDir, "versions", b.Name)
	binPath := filepath.Join(binDir, b.Name)
	versionMarker := filepath.Join(versionsDir, b.Version)

	defer func() {
		if err == nil {
			if verifyErr := verify(b, binPath); verifyErr != nil {
				err = fmt.Errorf("checksum verification failed: %v", verifyErr)
			}
		}
	}()

	// If version marker exists, assume binary is already installed.
	if cached(binPath, versionMarker) {
		return binPath, nil
	}

	// Ensure the cache directories exist.
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create bin directory: %v", err)
	}
	if err := os.MkdirAll(versionsDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create versions directory: %v", err)
	}

	if b.goPkg() {
		if err := goInstall(ctx, b, binDir); err != nil {
			return "", fmt.Errorf("failed to install Go tool: %v", err)
		}
		if err := markVersion(versionMarker); err != nil {
			return "", err
		}
		return binPath, nil
	} else {
		if err := binInstall(ctx, client, b, binPath); err != nil {
			return "", fmt.Errorf("failed to install binary: %v", err)
		}
	}

	if err := markVersion(versionMarker); err != nil {
		return "", err
	}

	return binPath, nil
}

func markVersion(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create version marker: %v", err)
	}

	return f.Close()
}

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

	cmd := exec.CommandContext(ctx, goBin, "install", packageName)

	binDir, err = filepath.Abs(binDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for bin directory %s: %w", binDir, err)
	}
	cmd.Env = append(os.Environ(), "GOBIN="+binDir)

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
		return fmt.Errorf("download failed: %v", err)
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
	if err != nil {
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
		return err
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}

	return nil
}

func findBinary(fsys fs.FS, name string) (_ fs.File, err error) {
	if ffs, ok := fsys.(archives.FileFS); ok {
		return ffs.Open(".")
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

func verify(b *bin, binPath string) error {
	if b.Checksum == "" {
		return nil
	}

	expected := b.Checksum

	actual, err := checksum(binPath)
	if err != nil {
		return fmt.Errorf("verify: %v", err)
	}

	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("verify: checksum mismatch for binary %q: expected %q but got %q", b.Name, expected, actual)
	}

	return nil
}

func checksum(filePath string) (string, error) {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "nil", fmt.Errorf("checksum: file does not exist")
	} else if err != nil {
		return "nil", fmt.Errorf("checksum: failed to stat file: %v", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "nil", fmt.Errorf("checksum: failed to open file %v", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "nil", fmt.Errorf("checksum: failed to hash file content %v", err)
	}

	hash := h.Sum(nil)

	return hex.EncodeToString(hash), nil
}
