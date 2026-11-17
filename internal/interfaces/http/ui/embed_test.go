package ui

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFS(t *testing.T) {
	sub := FS()
	require.NotNil(t, sub)
	_, err := fs.Stat(sub, ".")
	assert.NoError(t, err, "the embedded dist/ root must exist even when empty")
}
