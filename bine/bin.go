package bine

import (
	"fmt"
	"net/url"
	"strings"
)

type bin struct {
	Name    string `json:"name"`
	Version string `json:"version"`

	// Fields for asset-based downloads.
	URL          string `json:"url,omitempty"`
	AssetPattern string `json:"asset_pattern,omitempty"`

	// Field for go-based installs.
	GoPackage string `json:"go_package,omitempty"`

	// asset is computed by the namer when the config is loaded.
	asset string
}

func (b bin) goPkg() bool {
	return b.GoPackage != ""
}

func (b bin) downloadURL() (string, error) {
	parsedURL, err := url.Parse(b.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", b.URL, err)
	}

	var downloadURL string
	if strings.Contains(parsedURL.Host, "github.com") {
		downloadURL = fmt.Sprintf("%s/releases/download/v%s/%s", b.URL, b.Version, b.asset)
	} else {
		downloadURL = parsedURL.JoinPath(b.asset).String()
	}

	return downloadURL, nil
}
