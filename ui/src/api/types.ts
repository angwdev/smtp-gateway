export type Scope = 'read' | 'write';
export type MessageStatus = 'sent' | 'failed' | 'rejected';
export type AuditResult = 'ok' | 'denied' | 'error';
export type ActorType = 'user' | 'token' | 'cli' | 'system' | 'smtp';

export interface Session {
  auth: 'session' | 'token';
  user: { email: string; name: string } | null;
  token_name?: string;
  scope: Scope;
  expires_at: string | null;
}

export interface Credential {
  id: number;
  username: string;
  description: string;
  allowed_senders: string[];
  rate_per_hour: number;
  enabled: boolean;
  created_at: string;
  created_by: string;
  updated_at: string;
  last_used_at: string | null;
  last_used_ip: string;
  sent_count: number;
}

export interface CredentialCreate {
  username: string;
  description: string;
  allowed_senders: string[];
  rate_per_hour: number;
  enabled: boolean;
}
export type CredentialPatch = Partial<Omit<CredentialCreate, 'username'>>;
export interface CredentialSecret { credential: Credential; secret: string }

export interface Message {
  id: number;
  created_at: string;
  credential_id: number | null;
  username: string;
  remote_ip: string;
  mail_from: string;
  rcpt_to: string[];
  size: number;
  status: MessageStatus;
  detail: string;
  subject: string;
}
export interface HeaderField { name: string; value: string }
export interface MessageDetail extends Message {
  headers: HeaderField[];
  headers_raw: string;
  headers_truncated: boolean;
}
export interface MessageFilter {
  credential_id?: number;
  status?: MessageStatus;
  q?: string;
  from?: string;
  to?: string;
}

export interface AuditChange { from: unknown; to: unknown }
export interface AuditEvent {
  id: number;
  created_at: string;
  actor: string;
  actor_type: ActorType;
  action: string;
  target_type: string;
  target_id: number | null;
  target_label: string;
  result: AuditResult;
  ip: string;
  user_agent: string;
  request_id: string;
  detail: string;
  changes: Record<string, AuditChange>;
}
export interface AuditFilter {
  actor?: string;
  action?: string;
  target_type?: string;
  result?: AuditResult;
  q?: string;
  from?: string;
  to?: string;
}

export interface Token {
  id: number;
  name: string;
  hint: string;
  scope: Scope;
  created_at: string;
  created_by: string;
  expires_at: string | null;
  last_used_at: string | null;
  last_used_ip: string;
  revoked_at: string | null;
}
export interface TokenCreate { name: string; scope: Scope; expires_at: string | null }
export interface TokenSecret { token: Token; secret: string }

export interface HourCount { hour: string; sent: number; failed: number; rejected: number }
export interface Overview {
  counts_24h: Record<MessageStatus, number>;
  series: HourCount[];
  credentials: { total: number; active: number };
  upstream: { addr: string; tls: string; username: string; envelope_from: string };
  certificate: {
    source: string;
    label: string;
    names: string[];
    not_after: string | null;
    days_left: number;
    level: 'ok' | 'warn' | 'bad';
    acme: { renewing: boolean; ari: boolean; next_check: string | null; last_error: string } | null;
  };
  client_settings: { hostname: string; submission_port: string; smtps_port: string };
  version: string;
}
export interface UpstreamTest { ok: boolean; message: string; duration_ms: number }
export interface Page<T> { items: T[]; next_before: number | null }
