package bine

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestBinDownloadURL(t *testing.T) {
	t.Parallel()

	b := &bin{
		Name:         "test",
		Version:      "1.0.0",
		URL:          "https://example.com/downloads",
		AssetPattern: "{{ name }}_{{ version }}_{{ goos }}_{{ goarch }}",
		asset:        "test_1.0.0_linux_amd64",
	}

	u, err := b.downloadURL()
	assert.NilError(t, err)
	assert.Equal(t, u, "https://example.com/downloads/test_1.0.0_linux_amd64")

	b = &bin{
		Name:         "bine",
		Version:      "0.8.0",
		URL:          "https://github.com/artefactual-labs/bine",
		AssetPattern: "{{ name }}_{{ version }}_{{ goos }}_{{ goarch }}",
		asset:        "bine_0.8.0_linux_amd64",
	}

	u, err = b.downloadURL()
	assert.NilError(t, err)
	assert.Equal(t, u, "https://github.com/artefactual-labs/bine/releases/download/v0.8.0/bine_0.8.0_linux_amd64")
}
