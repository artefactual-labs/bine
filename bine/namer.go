package bine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

var (
	goos   = runtime.GOOS
	goarch = runtime.GOARCH
)

// namer computes the asset names defined in the configuration.
type namer struct {
	// uname operative system: `uname -s`, e.g. "Linux", "Darwin"...
	unameOS string
	// uname machine hardware name: `uname -m`, e.g. "x86_64", "arm64"...
	unameArch string
	// rustc target triple: `rustc -vV | sed -n -e 's/^host: //p'`, e.g. "x86_64-unknown-linux-gnu"
	triple string
}

func createNamer() (*namer, error) {
	n := namer{}

	ctx := context.TODO()

	out, err := execCommand(ctx, "uname", "-s").Output()
	if err != nil {
		return nil, fmt.Errorf("uname: %v", err)
	}
	n.unameOS = strings.TrimSpace(string(out))

	out, err = execCommand(ctx, "uname", "-m").Output()
	if err != nil {
		return nil, fmt.Errorf("uname: %v", err)
	}
	n.unameArch = strings.TrimSpace(string(out))

	if t := triple(ctx); t == "" {
		return nil, errors.New("unable to determine rustc target triple")
	} else {
		n.triple = t
	}

	return &n, nil
}

func (n *namer) run(bins []*bin) {
	if n == nil {
		return
	}
	for _, b := range bins {
		if b.goPkg() {
			continue
		}
		asset := b.AssetPattern
		asset = strings.ReplaceAll(asset, "{name}", b.Name)
		asset = strings.ReplaceAll(asset, "{version}", b.unprefixedVersion())
		asset = strings.ReplaceAll(asset, "{goos}", n.applyModifier(b, "goos", goos))
		asset = strings.ReplaceAll(asset, "{goarch}", n.applyModifier(b, "goarch", goarch))
		asset = strings.ReplaceAll(asset, "{os}", n.applyModifier(b, "os", n.unameOS))
		asset = strings.ReplaceAll(asset, "{arch}", n.applyModifier(b, "arch", n.unameArch))
		asset = strings.ReplaceAll(asset, "{triple}", n.triple)

		b.asset = asset
	}
}

// applyModifier applies template variable modifiers if they exist for the
// given variable, e.g.: when expanding {goos}, the user chooses to replaceAdd commentMore actions
// "darwin" with "osx".
func (n *namer) applyModifier(b *bin, variable, originalValue string) string {
	if b.Modifiers == nil {
		return originalValue
	} else if modifierMap, exists := b.Modifiers[variable]; !exists {
		return originalValue
	} else if modifiedValue, exists := modifierMap[originalValue]; exists {
		return modifiedValue
	}

	return originalValue
}

func triple(ctx context.Context) string {
	// First try to get triple from rustc.
	out, err := execCommand(ctx, "rustc", "-vV").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.HasPrefix(line, "host: ") {
				triple := strings.TrimSpace(strings.TrimPrefix(line, "host: "))
				if triple != "" {
					return triple
				}
			}
		}
	}

	// Inline arch mapping
	goarch := runtime.GOARCH
	arch := goarch
	switch goarch {
	case "amd64", "x86_64":
		arch = "x86_64"
	case "386", "i386", "i686":
		arch = "i686"
	case "arm64", "aarch64":
		arch = "aarch64"
	case "armv7l":
		arch = "armv7"
	case "arm":
		if v, _ := strconv.Atoi(os.Getenv("GOARM")); v >= 7 {
			arch = "armv7"
		} else {
			arch = "arm"
		}
	case "ppc64":
		arch = "powerpc64"
	case "ppc64le":
		arch = "powerpc64le"
	case "s390x":
		arch = "s390x"
	case "riscv64":
		arch = "riscv64"
	}

	// Inline vendor mapping
	goos := runtime.GOOS
	vendor := "unknown"
	switch goos {
	case "darwin":
		vendor = "apple"
	case "windows":
		vendor = "pc"
	case "android":
		vendor = "linux"
	}

	// Inline system mapping
	sys := goos
	switch goos {
	case "darwin":
		sys = "darwin"
	case "windows":
		sys = "windows"
	case "linux":
		sys = "linux"
	case "android":
		sys = "android"
	case "freebsd":
		sys = "freebsd"
	case "openbsd":
		sys = "openbsd"
	case "netbsd":
		sys = "netbsd"
	case "dragonfly":
		sys = "dragonfly"
	case "illumos":
		sys = "illumos"
	}

	// Inline abi mapping
	abi := ""
	switch goos {
	case "windows":
		abi = "msvc"
	case "linux":
		// Inline musl detection
		isMusl := false
		if _, err := os.Stat("/etc/alpine-release"); err == nil {
			isMusl = true
		} else {
			dirs := []string{"/lib", "/usr/lib", "/lib64", "/usr/lib64"}
			found := false
			for _, d := range dirs {
				_ = filepath.WalkDir(d, func(path string, _ os.DirEntry, _ error) error {
					if strings.HasPrefix(filepath.Base(path), "ld-musl") {
						found = true
						return fmt.Errorf("found-musl")
					}
					return nil
				})
				if found {
					isMusl = true
					break
				}
			}
			if !isMusl {
				out, err := exec.Command("ldd", "--version").CombinedOutput()
				if err == nil && bytes.Contains(out, []byte("musl")) {
					isMusl = true
				}
			}
		}
		if isMusl {
			abi = "musl"
		} else {
			abi = "gnu"
		}
	}

	// Assemble the triple; omit empty vendor or abi to avoid double dashes.
	triple := arch
	if vendor != "" {
		triple += "-" + vendor
	}
	triple += "-" + sys
	if abi != "" {
		triple += "-" + abi
	}
	return triple
}
