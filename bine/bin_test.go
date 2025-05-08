package bine

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestBin(t *testing.T) {
	t.Parallel()

	t.Run("Returns canonical version", func(t *testing.T) {
		t.Parallel()

		b := bin{}
		assert.Equal(t, b.canonicalVersion(), "")

		b = bin{Version: "1.2.3"}
		assert.Equal(t, b.canonicalVersion(), "v1.2.3")

		b = bin{Version: "v1.2.3"}
		assert.Equal(t, b.canonicalVersion(), "v1.2.3")
	})
}
