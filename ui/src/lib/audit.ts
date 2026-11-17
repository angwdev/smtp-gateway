import type { AuditChange, AuditEvent } from '../api/types';

const quoted = (label: string): string => (label ? ` ${label}` : '');

// What the actor did, as the end of a sentence that starts with the actor.
const SENTENCES: Record<string, (e: AuditEvent) => string> = {
  'session.login': () => 'signed in',
  'session.login_denied': () => 'was refused sign-in',
  'session.logout': () => 'signed out',
  'credential.create': (e) => `created login${quoted(e.target_label)}`,
  'credential.update': (e) => `changed login${quoted(e.target_label)}`,
  'credential.enable': (e) => `turned on login${quoted(e.target_label)}`,
  'credential.disable': (e) => `turned off login${quoted(e.target_label)}`,
  'credential.rotate': (e) => `replaced the password of login${quoted(e.target_label)}`,
  'credential.delete': (e) => `deleted login${quoted(e.target_label)}`,
  'token.create': (e) => `created API token${quoted(e.target_label)}`,
  'token.revoke': (e) => `revoked API token${quoted(e.target_label)}`,
  'token.auth_failed': () => 'sent an API token that was not accepted',
  'certificate.renew': () => 'asked for a certificate renewal',
  'upstream.test': () => 'tested the connection to the upstream server',
  'smtp.auth_failed': (e) => `failed to sign in over SMTP${e.target_label ? ` as ${e.target_label}` : ''}`,
  'smtp.lockout': () => 'was blocked from SMTP after repeated failed sign-ins',
  'api.call': () => 'called the API',
};

export function describeEvent(e: AuditEvent): string {
  return SENTENCES[e.action]?.(e) ?? e.action;
}

// Options of the "What" filter. A value ending in a dot matches a whole group.
export const ACTION_FILTERS: { value: string; label: string }[] = [
  { value: '', label: 'Everything' },
  { value: 'session.', label: 'Panel sign-ins' },
  { value: 'credential.', label: 'Changes to logins' },
  { value: 'token.', label: 'API tokens' },
  { value: 'smtp.', label: 'SMTP sign-in failures' },
  { value: 'api.call', label: 'API calls' },
  { value: 'upstream.test', label: 'Upstream tests' },
  { value: 'certificate.renew', label: 'Certificate renewals' },
];

export const ACTOR_WORD: Record<AuditEvent['actor_type'], string> = {
  user: 'Person',
  token: 'API token',
  cli: 'Command line',
  system: 'Gateway',
  smtp: 'SMTP client',
};

export const RESULT_WORD: Record<AuditEvent['result'], string> = { ok: 'Done', denied: 'Refused', error: 'Failed' };

const FIELD_LABEL: Record<string, string> = {
  description: 'Description',
  allowed_senders: 'Allowed senders',
  rate_per_hour: 'Hourly limit',
  enabled: 'Enabled',
  scope: 'Access',
  expires_at: 'Expires',
  username: 'Username',
};

function show(field: string, value: unknown): string {
  if (field === 'enabled') return value ? 'Yes' : 'No';
  if (field === 'rate_per_hour') return value ? `${String(value)} per hour` : 'No limit';
  if (field === 'allowed_senders') return Array.isArray(value) && value.length > 0 ? value.join(', ') : 'Any sender';
  if (field === 'scope') return value === 'write' ? 'Read and change' : value === 'read' ? 'Read only' : '(none)';
  if (field === 'expires_at') return value ? String(value) : 'Never';
  if (value === null || value === undefined || value === '') return '(empty)';
  return String(value);
}

export interface ChangeRow { label: string; before: string; after: string }

export function changeRows(changes: Record<string, AuditChange>): ChangeRow[] {
  return Object.entries(changes).map(([field, change]) => ({
    label: FIELD_LABEL[field] ?? field,
    before: show(field, change.from),
    after: show(field, change.to),
  }));
}
