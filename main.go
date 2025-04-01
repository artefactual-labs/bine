package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/mholt/archives"
)

const usage = `Usage:
    bine get [NAME]
    bine run [NAME]
    bine path

Examples:
    $ bine get golangci-lint
    /home/username/.cache/bine/linux/amd64/bin/golangci-lint

    $ bine run golangci-lint
    ...

    $ bine path
    /home/username/.cache/bine/linux/amd64/bin`

func main() {
	flag.Usage = func() { fmt.Fprintf(os.Stderr, "%s\n", usage) }
	flag.Parse()

	cmdArg := flag.Arg(0)
	binArg := flag.Arg(1)
	if cmdArg == "" {
		flag.Usage()
		os.Exit(1)
	}
	if cmdArg != "run" && cmdArg != "get" && cmdArg != "path" {
		flag.Usage()
		os.Exit(1)
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	cacheDir, err := cacheDir(cfg.Project)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if cmdArg == "path" {
		fmt.Println(filepath.Join(cacheDir, "bin"))
		return
	}

	if binArg == "" {
		flag.Usage()
		os.Exit(1)
	}
	var b *bin
	for _, item := range cfg.Bins {
		if item.Name == binArg {
			b = item
		}
	}
	if b == nil {
		fmt.Println("Command not found.")
		os.Exit(1)
	}

	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 3

	path, err := download(client.StandardClient(), b, cacheDir)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if cmdArg == "get" {
		fmt.Println(path)
		return
	}

	err = runTool(path, os.Args[3:])
	if err != nil {
		os.Exit(1)
	}
}

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

func download(client *http.Client, b *bin, cacheDir string) (string, error) {
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

	downloadURL, err := b.downloadURL()
	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %v", err)
	}

	// Download the asset.
	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download asset from %q: %v", downloadURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status %s", resp.Status)
	}
	f, err := os.CreateTemp("", "downloaded-*-"+filepath.Base(downloadURL))
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", fmt.Errorf("failed to write to temporary file: %w", err)
	}
	defer f.Close()

	// Reset file pointer to the beginning.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return "", fmt.Errorf("failed to reset file pointer: %v", err)
	}

	if err := extract(ctx, f, binPath); err != nil {
		return "", fmt.Errorf("download failed: %v", err)
	}

	// Create version marker file to indicate this version is installed.
	markerFile, err := os.Create(versionMarker)
	if err != nil {
		return "", fmt.Errorf("failed to create version marker: %v", err)
	}
	markerFile.Close()

	return binPath, nil
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
	defer dest.Close()

	// Copy the contents of the extracted file to the destination.
	if _, err := io.Copy(dest, f); err != nil {
		return err
	}

	if err := os.Chmod(binPath, 0o755); err != nil {
		return err
	}

	return nil
}

func runTool(path string, args []string) error {
	cmd := &exec.Cmd{
		Path:   path,
		Args:   args,
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	err := cmd.Start()
	if err == nil {
		c := make(chan os.Signal, 100)
		signal.Notify(c)
		go func() {
			for sig := range c {
				cmd.Process.Signal(sig)
			}
		}()
		err = cmd.Wait()
		signal.Stop(c)
		close(c)
	}
	if err != nil {
		if e, ok := err.(*exec.ExitError); !ok || !e.Exited() {
			fmt.Fprint(os.Stderr, err)
			return err
		}
	}

	return nil
}
