// Package sqlite is the SQLite persistence adapter: connection setup,
// migrations and the GORM repositories behind the domain interfaces.
package sqlite

import (
	"errors"
	"strings"

	msqlite "modernc.org/sqlite"
)

// SQLite extended result codes for uniqueness violations.
const (
	codeConstraintPrimaryKey = 1555
	codeConstraintUnique     = 2067
)

// isUniqueViolation reports whether err is a UNIQUE or PRIMARY KEY conflict.
// GORM's sqlite error translator only understands the cgo driver, so the
// modernc error code is checked here, with the message as a fallback.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var se *msqlite.Error
	if errors.As(err, &se) && (se.Code() == codeConstraintUnique || se.Code() == codeConstraintPrimaryKey) {
		return true
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
