package bine

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestBinVersions(t *testing.T) {
	tests := []struct {
		version    string // Provided by the user (we want to be flexible).
		canonical  string
		usable     string
		unprefixed string
	}{
		{"1.2.3", "v1.2.3", "v1.2.3", "1.2.3"},
		{"v1.2.3", "v1.2.3", "v1.2.3", "1.2.3"},
		{"jq-1.7", "", "jq-1.7", "jq-1.7"},
		{"jq-1.7.1", "", "jq-1.7.1", "jq-1.7.1"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("Version %q", test.version), func(t *testing.T) {
			b := bin{Version: test.version}
			assert.Equal(t, b.canonicalVersion(), test.canonical)
			assert.Equal(t, b.usableVersion(), test.usable)
			assert.Equal(t, b.unprefixedVersion(), test.unprefixed)
		})
	}
}

func TestBinTag(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		tagPattern  string
		expectedTag string
	}{
		{"Default pattern", "1.2.3", "", "v1.2.3"},
		{"uv style: no prefix", "0.7.11", "{version}", "0.7.11"},
		{"jq style: prefix", "1.8.0", "jq-{version}", "jq-1.8.0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			b := bin{
				Version:    test.version,
				TagPattern: test.tagPattern,
			}
			assert.Equal(t, b.tag(), test.expectedTag)
		})
	}
}
