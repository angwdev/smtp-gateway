package shared

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvalidIsAValidationError(t *testing.T) {
	err := fmt.Errorf("create credential: %w", Invalid("username", "Use 1-64 letters, digits or . _ - + @"))
	var ve *ValidationError
	require.True(t, errors.As(err, &ve))
	assert.Equal(t, "username", ve.Field)
	assert.Equal(t, "Use 1-64 letters, digits or . _ - + @", ve.Message)
	assert.Contains(t, err.Error(), "username")
}
