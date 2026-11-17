-- +goose Up
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

-- +goose Down
-- The baseline is not reversible: dropping these tables would delete every credential.
SELECT 1;
