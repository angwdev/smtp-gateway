-- +goose Up
ALTER TABLE messages ADD COLUMN subject TEXT NOT NULL DEFAULT '';
CREATE INDEX messages_status_id ON messages(status, id);

CREATE TABLE message_headers (
	message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
	raw        TEXT NOT NULL,
	truncated  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE audit_events (
	id           INTEGER PRIMARY KEY,
	created_at   INTEGER NOT NULL,
	actor        TEXT NOT NULL,
	actor_type   TEXT NOT NULL DEFAULT 'user',
	action       TEXT NOT NULL,
	target_type  TEXT NOT NULL DEFAULT '',
	target_id    INTEGER,
	target_label TEXT NOT NULL DEFAULT '',
	result       TEXT NOT NULL DEFAULT 'ok',
	ip           TEXT NOT NULL DEFAULT '',
	user_agent   TEXT NOT NULL DEFAULT '',
	request_id   TEXT NOT NULL DEFAULT '',
	detail       TEXT NOT NULL DEFAULT '',
	changes      TEXT NOT NULL DEFAULT ''
);
INSERT INTO audit_events (id, created_at, actor, actor_type, action, target_type, target_label, detail)
SELECT
	id,
	created_at,
	actor,
	CASE WHEN actor = 'cli' THEN 'cli' ELSE 'user' END,
	CASE action WHEN 'login' THEN 'session.login' WHEN 'logout' THEN 'session.logout'
	            WHEN 'tls.renew' THEN 'certificate.renew' ELSE action END,
	CASE WHEN action LIKE 'credential.%' THEN 'credential'
	     WHEN action = 'tls.renew' THEN 'certificate'
	     WHEN action IN ('login', 'logout') THEN 'session' ELSE '' END,
	CASE WHEN action LIKE 'credential.%' THEN substr(detail, 1, instr(detail || ' ', ' ') - 1) ELSE '' END,
	CASE WHEN action LIKE 'credential.%' AND action <> 'credential.update' THEN '' ELSE detail END
FROM audit;
DROP TABLE audit;
CREATE INDEX audit_events_created ON audit_events(created_at);
CREATE INDEX audit_events_action_id ON audit_events(action, id);
CREATE INDEX audit_events_actor_id ON audit_events(actor, id);

CREATE TABLE api_tokens (
	id           INTEGER PRIMARY KEY,
	name         TEXT NOT NULL UNIQUE COLLATE NOCASE,
	token_hash   TEXT NOT NULL UNIQUE,
	hint         TEXT NOT NULL,
	scope        TEXT NOT NULL CHECK (scope IN ('read', 'write')),
	created_at   INTEGER NOT NULL,
	created_by   TEXT NOT NULL DEFAULT '',
	expires_at   INTEGER,
	last_used_at INTEGER,
	last_used_ip TEXT NOT NULL DEFAULT '',
	revoked_at   INTEGER
);

CREATE TABLE sessions (
	id_hash    TEXT PRIMARY KEY,
	email      TEXT NOT NULL,
	name       TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	ip         TEXT NOT NULL DEFAULT '',
	user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_expires ON sessions(expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE api_tokens;
CREATE TABLE audit (
	id         INTEGER PRIMARY KEY,
	created_at INTEGER NOT NULL,
	actor      TEXT NOT NULL,
	action     TEXT NOT NULL,
	detail     TEXT NOT NULL DEFAULT ''
);
INSERT INTO audit (id, created_at, actor, action, detail)
	SELECT id, created_at, actor, action, CASE WHEN detail = '' THEN target_label ELSE detail END FROM audit_events;
CREATE INDEX audit_created ON audit(created_at);
DROP TABLE audit_events;
DROP TABLE message_headers;
DROP INDEX messages_status_id;
ALTER TABLE messages DROP COLUMN subject;
