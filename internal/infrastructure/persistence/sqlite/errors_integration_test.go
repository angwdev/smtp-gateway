//go:build integration

package sqlite

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsUniqueViolationFallsBackToTheMessage(t *testing.T) {
	// GORM's sqlite error translator only understands the cgo driver, and
	// not every error reaching this helper is a *msqlite.Error (errors.As
	// fails), so a plain error whose message still names the violation
	// must be recognised too.
	assert.True(t, isUniqueViolation(errors.New("UNIQUE constraint failed: credentials.username")))
	assert.False(t, isUniqueViolation(errors.New("some other failure")))
	assert.False(t, isUniqueViolation(nil))
}
