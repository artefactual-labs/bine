package bine

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"

	"github.com/artefactual-labs/bine/internal/packslip"
)

func (s *packslipSource) install(ctx context.Context, request installRequest, target string) (_ installResult, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("failed to install binary: %w", err)
		}
	}()
	b := request.effectiveBin()
	resolver, err := s.resolver(ctx)
	if err != nil {
		return installResult{}, err
	}
	release, err := resolver.Resolve(ctx, b.packslipRequest())
	if err != nil {
		return installResult{}, fmt.Errorf("resolve artifact: %w", err)
	}
	if err := checkPackslipRequirements(ctx, resolver.Host, release.Artifact.Requires, s.logger); err != nil {
		return installResult{}, err
	}
	if release.Project != b.Packslip.Project {
		s.logger.Info("Packslip repository was renamed.", "requested", b.Packslip.Project, "signed", release.Project)
	}
	client := *s.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("unsafe or excessive artifact redirect")
		}
		req.Header.Del("Authorization")
		return nil
	}
	body, err := openDownload(ctx, &client, release.Artifact.URL)
	if err != nil {
		return installResult{}, err
	}
	defer body.Close()
	file, err := os.CreateTemp("", "downloaded-*")
	if err != nil {
		return installResult{}, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := downloadVerified(file, body, release); err != nil {
		return installResult{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return installResult{}, fmt.Errorf("failed to reset file pointer: %w", err)
	}
	staged, err := os.CreateTemp(filepath.Dir(target), ".bine-bin-install-*")
	if err != nil {
		return installResult{}, fmt.Errorf("create temporary binary: %w", err)
	}
	stagedPath := staged.Name()
	_ = staged.Close()
	defer os.Remove(stagedPath)
	if err := extractPackslip(ctx, file, stagedPath, release); err != nil {
		return installResult{}, fmt.Errorf("extract failed: %w", err)
	}
	sum, err := checksum(stagedPath)
	if err != nil {
		return installResult{}, err
	}
	// Preserve trust acceptance before publication, with a fresh check under the lock.
	if err := resolver.Accept(b.Packslip.Project, release); err != nil {
		return installResult{}, fmt.Errorf("remember Packslip trust: %w", err)
	}
	if err := replaceFile(stagedPath, target); err != nil {
		return installResult{}, fmt.Errorf("move installed binary: %w", err)
	}
	receipt := &packslipReceipt{Request: b.packslipRequest(), Release: *release, ExecutableSHA256: sum}
	return installResult{Packslip: receipt, ExpectedSHA256: sum}, nil
}

func downloadVerified(dst io.Writer, src io.Reader, release *packslip.Resolved) error {
	size := release.Artifact.Size
	if size == nil || *size < 0 || *size == int64(^uint64(0)>>1) {
		return errors.New("invalid artifact size")
	}
	h256, h512 := sha256.New(), sha512.New()
	n, err := io.Copy(io.MultiWriter(dst, h256, h512), io.LimitReader(src, *size+1))
	if err != nil {
		return err
	}
	if n != *size {
		return fmt.Errorf("packslip artifact size mismatch: expected %d, got %d", *size, n)
	}
	if hex.EncodeToString(h256.Sum(nil)) != release.Digests["sha256"] {
		return errors.New("packslip artifact SHA-256 mismatch")
	}
	if sum, ok := release.Digests["sha512"]; ok && hex.EncodeToString(h512.Sum(nil)) != sum {
		return errors.New("packslip artifact SHA-512 mismatch")
	}
	return nil
}

func extractPackslip(ctx context.Context, src *os.File, destination string, release *packslip.Resolved) error {
	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer dst.Close()
	if release.Artifact.Format == "raw" {
		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	} else {
		var format archives.Extractor
		var compression archives.Compression
		switch release.Artifact.Format {
		case "tar.xz":
			compression = archives.Xz{}
		case "tar.gz", "tgz":
			compression = archives.Gz{}
		case "tar.zst":
			compression = archives.Zstd{}
		case "tar.bz2":
			compression = archives.Bz2{}
		case "tar":
			format = archives.Tar{}
		case "zip":
			format = archives.Zip{}
		default:
			return errors.New("unsupported Packslip archive format")
		}
		if compression != nil {
			format = archives.CompressedArchive{Extraction: archives.Tar{}, Compression: compression}
		}
		found := false
		err := format.Extract(ctx, src, func(ctx context.Context, info archives.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Archives may prefix root-relative names with current-directory components.
			name := info.NameInArchive
			for strings.HasPrefix(name, "./") {
				name = strings.TrimPrefix(name, "./")
			}
			if name != release.Executable.Path {
				return nil
			}
			if found {
				return errors.New("duplicate Packslip executable path in archive")
			}
			if !info.Mode().IsRegular() || info.LinkTarget != "" {
				return errors.New("packslip executable must be a regular file, not a link")
			}
			found = true
			f, err := info.Open()
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = io.Copy(dst, f)
			return err
		})
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("declared executable %q is missing from archive", release.Executable.Path)
		}
	}
	if err := dst.Chmod(0o755); err != nil {
		return err
	}
	return dst.Close()
}
