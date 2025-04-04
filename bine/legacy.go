package bine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mholt/archives"
)

func cacheDir(project string) (string, error) {
	if project == "" {
		return "", fmt.Errorf("project name is empty")
	}

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	cachePath := filepath.Join(homeDir, ".cache", "bine", project, goos, goarch)

	return cachePath, nil
}

func cached(binPath, versionMarker string) bool {
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return false
	}

	if _, err := os.Stat(versionMarker); os.IsNotExist(err) {
		return false
	}

	return true
}

func ensureInstalled(client *http.Client, b *bin, cacheDir string) (string, error) {
	ctx := context.Background()

	binDir := filepath.Join(cacheDir, "bin")
	versionsDir := filepath.Join(cacheDir, "versions", b.Name)
	binPath := filepath.Join(binDir, b.Name)
	versionMarker := filepath.Join(versionsDir, b.Version)

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
	}

	downloadURL, err := b.downloadURL()
	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %v", err)
	}

	// Download the asset.
	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download asset from %q: %v", downloadURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status %s", resp.Status)
	}
	f, err := os.CreateTemp("", "downloaded-*-"+filepath.Base(downloadURL))
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("failed to write to temporary file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Reset file pointer to the beginning.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("failed to reset file pointer: %v", err)
	}

	if err := extract(ctx, f, binPath); err != nil {
		return "", fmt.Errorf("download failed: %v", err)
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

	if b.Version == "" {
		b.Version = "latest"
	}
	if b.Version != "latest" {
		b.Version = fmt.Sprintf("v%s", strings.TrimPrefix(b.Version, "v"))
	}

	packageName := fmt.Sprintf("%s@%s", b.GoPackage, b.Version)

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

func extract(ctx context.Context, osf *os.File, binPath string) error {
	fsys, err := archives.FileSystem(ctx, osf.Name(), osf)
	if err != nil {
		return fmt.Errorf("archives.FileSystem: %v", err)
	}

	var f fs.File

	if ffs, ok := fsys.(archives.FileFS); ok {
		f, err = ffs.Open(".")
		if err != nil {
			return fmt.Errorf("can't open binary file: %v", err)
		}
	} else {
		err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == ".git" {
				return fs.SkipDir
			}
			if !d.IsDir() && filepath.Base(path) == filepath.Base(binPath) {
				f, err = fsys.Open(path)
				if err != nil {
					return fmt.Errorf("can't open binary file: %v", err)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
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
