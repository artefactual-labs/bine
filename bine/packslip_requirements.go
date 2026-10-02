package bine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/artefactual-labs/bine/internal/packslip"
)

var (
	numericVersion  = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)
	versionInOutput = regexp.MustCompile(`\b[0-9]+\.[0-9]+(\.[0-9]+)*\b`)
)

// Requirements are checked after selection; they cannot resolve ambiguous
// builds. Unknown checks warn, missing loader requirements fail, missing
// commands warn. No publisher executable is run while checking requirements.
func checkPackslipRequirements(ctx context.Context, host packslip.Host, req packslip.Requirements, logger logr.Logger) error {
	probe := func(name string, args ...string) string {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		data, _ := execCommand(ctx, name, args...).CombinedOutput()
		return strings.TrimSpace(string(data))
	}
	checkMin := func(label, actual, min string, hard bool) error {
		if min == "" {
			return nil
		}
		comparison, known := compareRequirementVersion(actual, min)
		if !known {
			logger.Info("Could not check Packslip host requirement.", "requirement", label, "minimum", min)
			return nil
		}
		if comparison < 0 {
			if hard {
				return fmt.Errorf("packslip requires %s >= %s; host has %s", label, min, actual)
			}
			logger.Info("Packslip command requirement is not met.", "command", label, "minimum", min, "actual", actual)
		}
		return nil
	}
	if req.OSMin != "" {
		actual := ""
		switch host.OS {
		case "darwin":
			actual = probe("sw_vers", "-productVersion")
		case "linux":
			actual = versionInOutput.FindString(probe("uname", "-r"))
		}
		if err := checkMin("OS version", actual, req.OSMin, true); err != nil {
			return err
		}
	}
	if req.GlibcMin != "" {
		actual := ""
		if host.OS == "linux" && host.Libc == "gnu" {
			actual = versionInOutput.FindString(probe("getconf", "GNU_LIBC_VERSION"))
		}
		if err := checkMin("glibc", actual, req.GlibcMin, true); err != nil {
			return err
		}
	}
	if len(req.Libs) > 0 {
		cache := ""
		if host.OS == "linux" {
			cache = probe("ldconfig", "-p")
		}
		for _, lib := range req.Libs {
			known, found := false, false
			if host.OS == "linux" && filepath.Base(lib) == lib {
				known = strings.Contains(cache, "libs found in cache")
				for line := range strings.SplitSeq(cache, "\n") {
					fields := strings.Fields(line)
					if len(fields) > 0 && fields[0] == lib {
						found = true
					}
				}
				for _, dir := range filepath.SplitList(os.Getenv("LD_LIBRARY_PATH")) {
					if dir != "" {
						if _, err := os.Stat(filepath.Join(dir, lib)); err == nil {
							found = true
						}
					}
				}
			}
			// macOS's shared cache is not represented by filesystem existence.
			// Report an unknown result until a loader-aware probe is available.
			if known && !found {
				return fmt.Errorf("packslip requires shared library %s; install it before retrying", lib)
			}
			if !found {
				logger.Info("Could not check Packslip shared library requirement.", "library", lib)
			}
		}
	}
	for _, cmd := range req.Bin {
		if _, err := exec.LookPath(cmd.Name); err != nil {
			logger.Info("Packslip requires a command that is not on PATH.", "command", cmd.Name, "minimum", cmd.Min)
			continue
		}
		if cmd.Min != "" {
			_ = checkMin(cmd.Name, versionInOutput.FindString(probe(cmd.Name, "--version")), cmd.Min, false)
		}
	}
	return nil
}

func compareRequirementVersion(actual, min string) (int, bool) {
	if !numericVersion.MatchString(actual) || !numericVersion.MatchString(min) {
		return 0, false
	}
	a, b := strings.Split(actual, "."), strings.Split(min, ".")
	for i := 0; i < max(len(a), len(b)); i++ {
		var x, y uint64
		var err error
		if i < len(a) {
			x, err = strconv.ParseUint(a[i], 10, 64)
			if err != nil {
				return 0, false
			}
		}
		if i < len(b) {
			y, err = strconv.ParseUint(b[i], 10, 64)
			if err != nil {
				return 0, false
			}
		}
		if x < y {
			return -1, true
		}
		if x > y {
			return 1, true
		}
	}
	return 0, true
}
