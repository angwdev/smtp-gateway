// Package store persists credentials, the message log and the panel audit log
// in a single SQLite file.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidLogin = errors.New("invalid credentials")
	ErrDuplicate    = errors.New("username already exists")
	ErrBadUsername  = errors.New("username must be 1-64 characters: letters, digits, . _ - + @")
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._+@-]{1,64}$`)

type Credential struct {
	ID             int64
	Username       string
	Description    string
	AllowedSenders []string // empty = any sender
	RatePerHour    int      // 0 = unlimited
	Enabled        bool
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      time.Time
	LastUsedAt     *time.Time
	LastUsedIP     string
	SentCount      int64
}

type Message struct {
	ID           int64
	CreatedAt    time.Time
	CredentialID *int64
	Username     string
	RemoteIP     string
	MailFrom     string
	RcptTo       []string
	Size         int64
	Status       string // sent | rejected | failed
	Detail       string
}

type AuditEntry struct {
	ID        int64
	CreatedAt time.Time
	Actor     string
	Action    string
	Detail    string
}

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS credentials (
	id              INTEGER PRIMARY KEY,
	username        TEXT NOT NULL UNIQUE COLLATE NOCASE,
	secret_hash     TEXT NOT NULL,
	description     TEXT NOT NULL DEFAULT '',
	allowed_senders TEXT NOT NULL DEFAULT '',
	rate_per_hour   INTEGER NOT NULL DEFAULT 0,
	enabled         INTEGER NOT NULL DEFAULT 1,
	created_at      INTEGER NOT NULL,
	created_by      TEXT NOT NULL DEFAULT '',
	updated_at      INTEGER NOT NULL,
	last_used_at    INTEGER,
	last_used_ip    TEXT NOT NULL DEFAULT '',
	sent_count      INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS messages (
	id            INTEGER PRIMARY KEY,
	created_at    INTEGER NOT NULL,
	credential_id INTEGER REFERENCES credentials(id) ON DELETE SET NULL,
	username      TEXT NOT NULL DEFAULT '',
	remote_ip     TEXT NOT NULL DEFAULT '',
	mail_from     TEXT NOT NULL DEFAULT '',
	rcpt_to       TEXT NOT NULL DEFAULT '',
	size          INTEGER NOT NULL DEFAULT 0,
	status        TEXT NOT NULL,
	detail        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS messages_created ON messages(created_at);
CREATE INDEX IF NOT EXISTS messages_cred_created ON messages(credential_id, created_at);
CREATE TABLE IF NOT EXISTS audit (
	id         INTEGER PRIMARY KEY,
	created_at INTEGER NOT NULL,
	actor      TEXT NOT NULL,
	action     TEXT NOT NULL,
	detail     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS audit_created ON audit(created_at);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection keeps memory low and serialises writes; every query here
	// is a small indexed lookup.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Setting returns a persisted value, creating it with gen() on first use.
func (s *Store) Setting(key string, gen func() string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err == nil {
		return v, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	v = gen()
	_, err = s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)`, key, v)
	return v, err
}

// ---- credentials -----------------------------------------------------------

// GenerateSecret returns a random 32-character password (~190 bits).
func GenerateSecret() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

// Passwords are always server-generated high-entropy secrets, so a fast hash
// is sufficient (as with API tokens) and keeps AUTH essentially free.
func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

var dummyHash = hashSecret("dummy-secret-for-timing")

func (s *Store) Authenticate(ctx context.Context, username, password string) (*Credential, error) {
	var hash string
	c, err := s.scanCredential(s.db.QueryRowContext(ctx, credSelect+` WHERE username = ?`, username), &hash)
	if errors.Is(err, ErrNotFound) {
		subtle.ConstantTimeCompare([]byte(dummyHash), []byte(hashSecret(password)))
		return nil, ErrInvalidLogin
	}
	if err != nil {
		return nil, err
	}
	if subtle.ConstantTimeCompare([]byte(hash), []byte(hashSecret(password))) != 1 || !c.Enabled {
		return nil, ErrInvalidLogin
	}
	return c, nil
}

const credSelect = `SELECT id, username, secret_hash, description, allowed_senders, rate_per_hour, enabled,
	created_at, created_by, updated_at, last_used_at, last_used_ip, sent_count FROM credentials`

type scanner interface{ Scan(...any) error }

func (s *Store) scanCredential(row scanner, hash *string) (*Credential, error) {
	var (
		c                Credential
		senders          string
		enabled          int
		created, updated int64
		lastUsed         sql.NullInt64
		discard          string
	)
	if hash == nil {
		hash = &discard
	}
	err := row.Scan(&c.ID, &c.Username, hash, &c.Description, &senders, &c.RatePerHour, &enabled,
		&created, &c.CreatedBy, &updated, &lastUsed, &c.LastUsedIP, &c.SentCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled != 0
	c.AllowedSenders = splitLines(senders)
	c.CreatedAt = time.Unix(created, 0)
	c.UpdatedAt = time.Unix(updated, 0)
	if lastUsed.Valid {
		t := time.Unix(lastUsed.Int64, 0)
		c.LastUsedAt = &t
	}
	return &c, nil
}

func (s *Store) ListCredentials(ctx context.Context) ([]*Credential, error) {
	rows, err := s.db.QueryContext(ctx, credSelect+` ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Credential
	for rows.Next() {
		c, err := s.scanCredential(rows, nil)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetCredential(ctx context.Context, id int64) (*Credential, error) {
	return s.scanCredential(s.db.QueryRowContext(ctx, credSelect+` WHERE id = ?`, id), nil)
}

// CreateCredential stores c and returns its generated password.
func (s *Store) CreateCredential(ctx context.Context, c *Credential) (string, error) {
	if !usernameRe.MatchString(c.Username) {
		return "", ErrBadUsername
	}
	secret := GenerateSecret()
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO credentials
		(username, secret_hash, description, allowed_senders, rate_per_hour, enabled, created_at, created_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Username, hashSecret(secret), c.Description, strings.Join(c.AllowedSenders, "\n"),
		c.RatePerHour, boolInt(c.Enabled), now, c.CreatedBy, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return "", ErrDuplicate
		}
		return "", err
	}
	c.ID, _ = res.LastInsertId()
	return secret, nil
}

func (s *Store) UpdateCredential(ctx context.Context, c *Credential) error {
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET description = ?, allowed_senders = ?,
		rate_per_hour = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		c.Description, strings.Join(c.AllowedSenders, "\n"), c.RatePerHour, boolInt(c.Enabled), time.Now().Unix(), c.ID)
	return affected(res, err)
}

func (s *Store) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET enabled = ?, updated_at = ? WHERE id = ?`,
		boolInt(enabled), time.Now().Unix(), id)
	return affected(res, err)
}

// RotateSecret replaces the password of credential id and returns the new one.
func (s *Store) RotateSecret(ctx context.Context, id int64) (string, error) {
	secret := GenerateSecret()
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET secret_hash = ?, updated_at = ? WHERE id = ?`,
		hashSecret(secret), time.Now().Unix(), id)
	if err := affected(res, err); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Store) DeleteCredential(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return affected(res, err)
}

func (s *Store) TouchCredential(ctx context.Context, id int64, ip string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE credentials SET last_used_at = ?, last_used_ip = ? WHERE id = ?`,
		time.Now().Unix(), ip, id)
	return err
}

// CountSince counts messages relayed (or attempted upstream) by a credential.
func (s *Store) CountSince(ctx context.Context, credID int64, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages
		WHERE credential_id = ? AND created_at >= ? AND status IN ('sent', 'failed')`, credID, since.Unix()).Scan(&n)
	return n, err
}

// ---- message log -----------------------------------------------------------

func (s *Store) LogMessage(ctx context.Context, m *Message) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO messages
		(created_at, credential_id, username, remote_ip, mail_from, rcpt_to, size, status, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), m.CredentialID, m.Username, m.RemoteIP, m.MailFrom,
		strings.Join(m.RcptTo, ", "), m.Size, m.Status, truncate(m.Detail, 500))
	if err != nil {
		return err
	}
	if m.Status == "sent" && m.CredentialID != nil {
		_, err = s.db.ExecContext(ctx, `UPDATE credentials SET sent_count = sent_count + 1 WHERE id = ?`, *m.CredentialID)
	}
	return err
}

type MessageFilter struct {
	CredentialID int64
	Status       string
	Query        string
	BeforeID     int64
	Limit        int
}

func (s *Store) ListMessages(ctx context.Context, f MessageFilter) ([]*Message, error) {
	q := `SELECT id, created_at, credential_id, username, remote_ip, mail_from, rcpt_to, size, status, detail
		FROM messages WHERE 1=1`
	var args []any
	if f.CredentialID > 0 {
		q += ` AND credential_id = ?`
		args = append(args, f.CredentialID)
	}
	if f.Status != "" {
		q += ` AND status = ?`
		args = append(args, f.Status)
	}
	if f.Query != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.Query) + "%"
		q += ` AND (mail_from LIKE ? ESCAPE '\' OR rcpt_to LIKE ? ESCAPE '\' OR username LIKE ? ESCAPE '\' OR detail LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like)
	}
	if f.BeforeID > 0 {
		q += ` AND id < ?`
		args = append(args, f.BeforeID)
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var (
			m       Message
			created int64
			credID  sql.NullInt64
			rcpts   string
		)
		if err := rows.Scan(&m.ID, &created, &credID, &m.Username, &m.RemoteIP, &m.MailFrom, &rcpts, &m.Size, &m.Status, &m.Detail); err != nil {
			return nil, err
		}
		m.CreatedAt = time.Unix(created, 0)
		if credID.Valid {
			m.CredentialID = &credID.Int64
		}
		if rcpts != "" {
			m.RcptTo = strings.Split(rcpts, ", ")
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// StatusCounts returns message counts per status since t.
func (s *Store) StatusCounts(ctx context.Context, since time.Time) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM messages WHERE created_at >= ? GROUP BY status`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// ---- audit log -------------------------------------------------------------

func (s *Store) Audit(ctx context.Context, actor, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit(created_at, actor, action, detail) VALUES (?, ?, ?, ?)`,
		time.Now().Unix(), actor, action, truncate(detail, 1000))
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]*AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, actor, action, detail FROM audit ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AuditEntry
	for rows.Next() {
		var a AuditEntry
		var created int64
		if err := rows.Scan(&a.ID, &created, &a.Actor, &a.Action, &a.Detail); err != nil {
			return nil, err
		}
		a.CreatedAt = time.Unix(created, 0)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// Prune deletes message and audit rows older than t.
func (s *Store) Prune(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE created_at < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	_, err = s.db.ExecContext(ctx, `DELETE FROM audit WHERE created_at < ?`, before.Unix())
	return n, err
}

// ---- helpers ---------------------------------------------------------------

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
