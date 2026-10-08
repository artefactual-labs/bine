package bine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"golang.org/x/mod/semver"
)

type goSource struct {
	client *http.Client
	logger logr.Logger
}

// Resolve through Go rather than guessing the module that owns a package.
// The temporary module isolates this lookup from the project's go.mod and go.work.
func goResolveVersion(ctx context.Context, packagePath string) (string, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "bine-go-resolve-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module bine.invalid/resolve\n"), 0o600); err != nil {
		return "", err
	}
	run := func(args ...string) ([]byte, error) {
		cmd := execCommand(ctx, goBin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GO111MODULE=on", "GOFLAGS=")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			// A wrapped exit error would make the CLI skip these diagnostics.
			return nil, fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
		return out, nil
	}
	if _, err := run("get", packagePath+"@latest"); err != nil {
		return "", err
	}
	out, err := run("list", "-json", packagePath)
	if err != nil {
		return "", err
	}
	var pkg struct {
		Name   string
		Module *struct{ Version string }
	}
	if err := json.Unmarshal(out, &pkg); err != nil {
		return "", err
	}
	if pkg.Name != "main" || pkg.Module == nil || semver.Canonical(pkg.Module.Version) == "" {
		return "", errors.New("package must be a Go executable with a resolved module version")
	}
	return strings.TrimPrefix(pkg.Module.Version, "v"), nil
}

func (s *goSource) install(ctx context.Context, request installRequest, target string) (installResult, error) {
	b := request.effectiveBin()
	if err := goInstall(ctx, b, filepath.Dir(target)); err != nil {
		return installResult{}, fmt.Errorf("failed to install Go tool: %w", err)
	}
	result := installResult{}
	if request.Bin.isLatest() {
		if request.VersionOverride != "" {
			result.ResolvedVersion = strings.TrimPrefix(b.usableVersion(), "v")
		} else if version, err := goInstalledVersion(ctx, target); err != nil {
			s.logger.V(1).Info("Could not determine installed version for 'latest' tracking.", "bin", b.Name, "err", err)
		} else {
			result.ResolvedVersion = version
		}
	}
	return result, nil
}

func (s *goSource) validateMarker(_ context.Context, _ *bin, marker *versionMarkerDocument) (bool, error) {
	return marker.Packslip == nil, nil
}

// latestVersion retrieves the latest version for a Go package binary.
//
// It uses the pkg.go.dev website to find the latest version of the package.
//
// TODO: investigate if we can use the Go module proxy instead, e.g. see how
// github.com/icholy/gomajor does it. But we may also need to extract the path
// of a module given something like "goa.design/goa/v3/cmd/goa"?
func (p *goSource) latestVersion(ctx context.Context, bin *bin) (string, error) {
	pkgPath := bin.GoPackage
	url := fmt.Sprintf("https://pkg.go.dev/%s?tab=versions", pkgPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %v", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pkg.go.dev returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %v", err)
	}

	// Extract version from HTML using regex to find the first js-versionLink.
	re := regexp.MustCompile(`<a class="js-versionLink"[^>]*>([^<]+)</a>`)
	matches := re.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("extract version: no match found in HTML")
	}

	latestVersion := string(matches[1])
	latestVersion = strings.TrimPrefix(latestVersion, "v")

	return latestVersion, nil
}

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
