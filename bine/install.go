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
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mholt/archives"
	"golang.org/x/mod/semver"
)

// Supporting functions for installing binaries.

// goInstall installs a Go tool using 'go install'.
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

	tmpBinDir, err := os.MkdirTemp(binDir, ".bine-go-install-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary bin directory: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpBinDir) }()

	cmd := execCommand(ctx, goBin, "install", packageName)

	// Set GOBIN to install the binary there. fakeExecCommand sets cmd.Env so
	// we can't assume it's empty.
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, "GOBIN="+tmpBinDir)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = "(no stderr output)"
		}
		return fmt.Errorf("`go install %s` failed: %v\nstderr: %s", packageName, err, msg)
	}

	installedPath := filepath.Join(tmpBinDir, defaultGoBinaryName(b.GoPackage))
	targetPath := filepath.Join(binDir, b.Name)
	if err := replaceFile(installedPath, targetPath); err != nil {
		return fmt.Errorf("move installed binary: %v", err)
	}

	if err := os.Chmod(targetPath, 0o755); err != nil {
		return fmt.Errorf("chmod installed binary: %v", err)
	}

	return nil
}

// goInstalledVersion returns the version of the Go module embedded in a binary
// by running "go version -m". This is used to determine the resolved version
// after installing a Go tool with @latest.
func goInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("cannot find 'go' command: %v", err)
	}

	cmd := execCommand(ctx, goBin, "version", "-m", binaryPath)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go version -m: %v", err)
	}

	// Parse "go version -m" output to find the "mod" line.
	// Example output:
	//   /path/to/binary: go1.21.0
	//           path    github.com/foo/bar/cmd/tool
	//           mod     github.com/foo/bar      v1.2.3  h1:...
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" {
			rawVersion := strings.TrimPrefix(fields[2], "v")
			// Validate that the extracted version is a proper semver.
			// Versions like "(devel)" or pseudo-versions are not useful for
			// upgrade comparisons.
			if semver.Canonical("v"+rawVersion) == "" {
				return "", fmt.Errorf("non-semver version %q reported by 'go version -m'", fields[2])
			}
			return rawVersion, nil
		}
	}

	return "", errors.New("could not determine installed version from 'go version -m' output")
}

func defaultGoBinaryName(pkg string) string {
	name := path.Base(pkg)
	for isGoMajorVersionPath(name) {
		pkg = path.Dir(pkg)
		name = path.Base(pkg)
	}
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

func isGoMajorVersionPath(name string) bool {
	if len(name) < 2 || name[0] != 'v' {
		return false
	}

	n, err := strconv.Atoi(name[1:])
	return err == nil && n >= 2
}

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
