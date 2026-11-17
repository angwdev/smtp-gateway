// Package credential holds the use cases for SMTP logins.
package credential

// CreateCommand creates a credential.
type CreateCommand struct {
	Username       string
	Description    string
	AllowedSenders []string
	RatePerHour    int
	Enabled        bool
	CreatedBy      string
}

// UpdateCommand changes a credential. Nil fields are left alone.
type UpdateCommand struct {
	ID             int64
	Description    *string
	AllowedSenders *[]string
	RatePerHour    *int
	Enabled        *bool
}

// AuthenticateCommand checks an SMTP login.
type AuthenticateCommand struct {
	Username string
	Password string
	IP       string
}
