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

INSERT INTO credentials (id, username, secret_hash, created_at, updated_at, sent_count) VALUES (1, 'nextcloud', 'aa11', 1792293143, 1792293143, 1);
INSERT INTO messages (id, created_at, credential_id, username, remote_ip, mail_from, rcpt_to, size, status, detail)
	VALUES (1, 1792293200, 1, 'nextcloud', '203.0.113.9', 'a@example.com', 'b@example.com', 1200, 'sent', '2.0.0 Ok');
INSERT INTO audit (id, created_at, actor, action, detail) VALUES
	(1, 1792293100, 'admin@example.com', 'login', ''),
	(2, 1792293143, 'cli', 'credential.create', 'nextcloud'),
	(3, 1792293300, 'admin@example.com', 'credential.update', 'nextcloud senders="" rate=5 enabled=true'),
	(4, 1792293400, 'admin@example.com', 'tls.renew', 'smtp.example.com');
INSERT INTO settings (key, value) VALUES ('session_key', 'abcd');
