package main

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

type bin struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Version      string `json:"version"`
	AssetPattern string `json:"asset_pattern"`
}

func (b bin) assetName() string {
	asset := b.AssetPattern
	asset = strings.ReplaceAll(asset, "{name}", b.Name)
	asset = strings.ReplaceAll(asset, "{version}", b.Version)
	asset = strings.ReplaceAll(asset, "{goos}", runtime.GOOS)
	asset = strings.ReplaceAll(asset, "{goarch}", runtime.GOARCH)

	return asset
}

func (b bin) downloadURL() (string, error) {
	parsedURL, err := url.Parse(b.URL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", b.URL, err)
	}
	if !strings.Contains(parsedURL.Host, "github.com") {
		return "", fmt.Errorf("unsupported host %q", parsedURL.Host)
	}

	downloadURL := fmt.Sprintf("%s/releases/download/v%s/%s", b.URL, b.Version, b.assetName())

	return downloadURL, nil
}
