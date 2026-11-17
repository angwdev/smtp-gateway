import type { Page } from '@playwright/test';

import type {
  AuditEvent, Credential, Message, MessageDetail, Overview, Session, Token,
} from '../src/api/types';

export const SESSION: Session = {
  auth: 'session',
  user: { email: 'ana@example.com', name: 'Ana Gomez' },
  scope: 'write',
  expires_at: null,
};

function hourSeries(): Overview['series'] {
  const now = new Date('2026-11-17T12:00:00Z');
  return Array.from({ length: 24 }, (_, i) => {
    const hour = new Date(now.getTime() - (23 - i) * 3600_000).toISOString().slice(0, 13) + ':00:00Z';
    // Morning peak at slot 8, two explicit zero hours (slots 2 and 20).
    if (i === 2 || i === 20) return { hour, sent: 0, failed: 0, rejected: 0 };
    if (i === 8) return { hour, sent: 40, failed: 2, rejected: 1 };
    return { hour, sent: 8, failed: 0, rejected: 0 };
  });
}

export const OVERVIEW: Overview = {
  counts_24h: { sent: 212, failed: 3, rejected: 1 },
  series: hourSeries(),
  credentials: { total: 6, active: 5 },
  upstream: { addr: 'mail.example.com:587', tls: 'starttls', username: 'relay@example.com', envelope_from: '' },
  certificate: {
    source: 'acme',
    label: "Let's Encrypt via Cloudflare DNS",
    names: ['smtp.example.com'],
    not_after: '2027-01-17T00:00:00Z',
    days_left: 61,
    level: 'ok',
    acme: { renewing: false, ari: true, next_check: '2026-11-18T00:00:00Z', last_error: '' },
  },
  client_settings: { hostname: 'smtp.example.com', submission_port: '587', smtps_port: '465' },
  version: '0.2.0',
};

const LONG_DESCRIPTION = 'Backs the nightly export job that mirrors invoices to the archive bucket every night at two';

export const CREDENTIALS: Credential[] = [
  { id: 11, username: 'nextcloud', description: 'File sync', allowed_senders: [], rate_per_hour: 0, enabled: true, created_at: '2026-09-01T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-09-01T09:00:00Z', last_used_at: '2026-11-17T09:50:00Z', last_used_ip: '203.0.113.9', sent_count: 180 },
  { id: 12, username: 'grafana', description: 'Alert e-mails', allowed_senders: ['@example.com'], rate_per_hour: 50, enabled: true, created_at: '2026-09-02T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-09-02T09:00:00Z', last_used_at: '2026-11-17T10:00:00Z', last_used_ip: '203.0.113.9', sent_count: 20 },
  { id: 13, username: 'leaked-app', description: 'Turned off after a leak', allowed_senders: [], rate_per_hour: 0, enabled: false, created_at: '2026-09-03T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-11-01T09:00:00Z', last_used_at: '2026-10-30T09:00:00Z', last_used_ip: '203.0.113.5', sent_count: 4 },
  { id: 14, username: 'nightly-export', description: LONG_DESCRIPTION, allowed_senders: ['@*.example.com'], rate_per_hour: 5, enabled: true, created_at: '2026-09-04T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-09-04T09:00:00Z', last_used_at: '2026-11-16T02:00:00Z', last_used_ip: '203.0.113.11', sent_count: 8 },
  { id: 15, username: 'never-used', description: 'Provisioned, not wired up yet', allowed_senders: [], rate_per_hour: 0, enabled: true, created_at: '2026-11-10T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-11-10T09:00:00Z', last_used_at: null, last_used_ip: '', sent_count: 0 },
  { id: 16, username: 'ci-notify', description: 'Build pipeline notifications', allowed_senders: [], rate_per_hour: 0, enabled: true, created_at: '2026-10-01T09:00:00Z', created_by: 'ana@example.com', updated_at: '2026-10-01T09:00:00Z', last_used_at: '2026-11-17T09:00:00Z', last_used_ip: '203.0.113.20', sent_count: 60 },
];

const LONG_SUBJECT = 'Weekly report for the week that had the migration, the outage, the retro, the fix and the follow-up fix: everything you need to know about it';
const LONG_ADDRESS = 'this-address-is-sixty-four-characters-long-for-testing-wrap@example.com';

function messages(): Message[] {
  const items: Message[] = [];
  const base = new Date('2026-11-17T12:00:00Z').getTime();
  for (let i = 0; i < 60; i += 1) {
    const id = 5001 + i;
    const createdAt = new Date(base - i * 55 * 60_000).toISOString();
    let status: Message['status'] = 'sent';
    let detail = '2.0.0 Ok: queued as 4F1A';
    let subject = `Message number ${i}`;
    let rcptTo = ['recipient@example.com'];
    let mailFrom = 'relay@example.com';

    if (i === 3) { status = 'failed'; detail = 'Upstream refused the message: 550 5.1.1 The e-mail account that you tried to reach does not exist. Please try double-checking the recipient address for typos or unnecessary spaces.'; }
    if (i === 7) { status = 'failed'; detail = 'Connection to mail.example.com:587 timed out after 30s while negotiating STARTTLS.'; }
    if (i === 11) { status = 'rejected'; detail = 'sender not allowed for this login'; }
    if (i === 20) subject = '';
    if (i === 25) { rcptTo = Array.from({ length: 12 }, (_, j) => `person${j}@example.com`); }
    if (i === 30) subject = LONG_SUBJECT;
    if (i === 35) mailFrom = LONG_ADDRESS;

    items.push({
      id, created_at: createdAt, credential_id: 11, username: 'nextcloud', remote_ip: '203.0.113.9',
      mail_from: mailFrom, rcpt_to: rcptTo, size: 4096 + i * 13, status, detail, subject,
    });
  }
  // All of these messages belong to login id 11 (used by the /logins/11 shot),
  // so none of their displayed times may show the hour 11 (e.g. "11:05"),
  // which the leaked-id check would mistake for the credential id.
  for (const item of items) {
    const created = new Date(item.created_at);
    if (created.getUTCHours() === 11) {
      created.setUTCHours(10);
      item.created_at = created.toISOString();
    }
  }
  return items;
}

function must<T>(value: T | undefined, what: string): T {
  if (value === undefined) throw new Error(`fixtures: missing ${what}`);
  return value;
}

export const MESSAGES: Message[] = messages();

export const MESSAGE_5001_DETAIL: MessageDetail = {
  ...must(MESSAGES[0], 'MESSAGES[0]'),
  headers: [
    { name: 'Received', value: 'from mail.example.com (mail.example.com [203.0.113.1])\r\n by smtp.example.com with ESMTPS id 4F1A;\r\n Tue, 17 Nov 2026 12:00:00 +0000' },
    { name: 'From', value: 'Nextcloud <relay@example.com>' },
    { name: 'To', value: 'recipient@example.com' },
    { name: 'Subject', value: 'Message number 0' },
    { name: 'Date', value: 'Tue, 17 Nov 2026 12:00:00 +0000' },
    { name: 'Message-ID', value: '<4f1a@smtp.example.com>' },
    { name: 'MIME-Version', value: '1.0' },
    { name: 'Content-Type', value: 'text/plain; charset=UTF-8' },
    { name: 'X-Mailer', value: 'Nextcloud Mail' },
    { name: 'X-Priority', value: '3' },
    { name: 'Return-Path', value: '<relay@example.com>' },
    { name: 'DKIM-Signature', value: 'v=1; a=rsa-sha256; d=example.com; s=default' },
    { name: 'X-Spam-Status', value: 'No, score=0.0' },
    { name: 'Reply-To', value: 'support@example.com' },
  ],
  headers_raw: 'Received: from mail.example.com\r\nFrom: Nextcloud <relay@example.com>\r\nTo: recipient@example.com\r\nSubject: Message number 0\r\nDate: Tue, 17 Nov 2026 12:00:00 +0000\r\nMessage-ID: <4f1a@smtp.example.com>\r\n',
  headers_truncated: false,
};

function auditEvents(): AuditEvent[] {
  const actions = [
    'session.login', 'session.login_denied', 'session.logout', 'credential.create', 'credential.update',
    'credential.enable', 'credential.disable', 'credential.rotate', 'credential.delete', 'token.create',
    'token.revoke', 'token.auth_failed', 'certificate.renew', 'upstream.test', 'smtp.auth_failed',
    'smtp.lockout', 'api.call',
  ] as const;
  const base = new Date('2026-11-17T12:00:00Z').getTime();
  const items: AuditEvent[] = [];
  for (let i = 0; i < 30; i += 1) {
    const action = must(actions[i % actions.length], 'action');
    const actorType: AuditEvent['actor_type'] = action.startsWith('smtp.') ? 'smtp' : action === 'api.call' ? 'token' : 'user';
    const actor = actorType === 'smtp' ? '203.0.113.50' : actorType === 'token' ? 'grafana' : 'ana@example.com';
    let result: AuditEvent['result'] = 'ok';
    if (i === 1) result = 'denied';
    if (i === 14) result = 'denied';
    if (i === 15) result = 'error';
    items.push({
      id: 1000 + i,
      created_at: new Date(base - i * 40 * 60_000).toISOString(),
      actor,
      actor_type: actorType,
      action,
      target_type: action.startsWith('credential') ? 'credential' : action.startsWith('token') ? 'token' : '',
      target_id: action.startsWith('credential') ? 11 : action.startsWith('token') ? 2 : null,
      target_label: action.startsWith('credential') ? 'nextcloud' : action.startsWith('token') ? 'grafana' : '',
      result,
      ip: actorType === 'user' ? '203.0.113.9' : actorType === 'smtp' ? '203.0.113.50' : '',
      user_agent: actorType === 'user' ? 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/120.0' : '',
      request_id: `req-fixture-${String(i + 1).padStart(4, '0')}`,
      detail: action === 'api.call' ? 'GET /messages?limit=5' : '',
      changes: i === 4 ? { description: { from: 'File sync', to: 'File sync (nc)' }, rate_per_hour: { from: 0, to: 20 }, enabled: { from: true, to: false } } : {},
    });
  }
  return items;
}

export const AUDIT: AuditEvent[] = auditEvents();

export const TOKENS: Token[] = [
  { id: 1, name: 'read-only-script', hint: 'a1B2', scope: 'read', created_at: '2026-10-01T00:00:00Z', created_by: 'ana@example.com', expires_at: null, last_used_at: '2026-11-17T09:00:00Z', last_used_ip: '203.0.113.30', revoked_at: null },
  { id: 2, name: 'grafana', hint: 'x9Qk', scope: 'write', created_at: '2026-10-05T00:00:00Z', created_by: 'ana@example.com', expires_at: '2027-01-01T23:59:59Z', last_used_at: '2026-11-17T08:00:00Z', last_used_ip: '203.0.113.31', revoked_at: null },
  { id: 3, name: 'old-deploy-hook', hint: 'z7Lp', scope: 'write', created_at: '2026-08-01T00:00:00Z', created_by: 'ana@example.com', expires_at: null, last_used_at: '2026-09-01T00:00:00Z', last_used_ip: '203.0.113.32', revoked_at: '2026-10-01T00:00:00Z' },
];

export interface MockOverrides { empty?: boolean }

function page1<T extends { id: number }>(items: T[], before?: number, limit = 50): { items: T[]; next_before: number | null } {
  const sorted = [...items].sort((a, b) => b.id - a.id);
  const start = before ? sorted.findIndex((item) => item.id < before) : 0;
  const slice = sorted.slice(start === -1 ? sorted.length : start, (start === -1 ? sorted.length : start) + limit);
  const next = slice.length === limit ? must(slice.at(-1), 'last item').id : null;
  return { items: slice, next_before: next };
}

// Registers page.route for every /api/v1/* endpoint the SPA calls, answering
// from the fixtures above (or empty data with the `empty` override).
export async function mockApi(page: Page, overrides: MockOverrides = {}): Promise<void> {
  const empty = Boolean(overrides.empty);

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace(/^.*\/api\/v1/, '');
    const method = request.method();
    const json = (status: number, body: unknown) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });

    if (path === '/session' && method === 'GET') return json(200, SESSION);
    if (path === '/session' && method === 'DELETE') return json(200, { redirect_url: '/signed-out' });
    if (path === '/overview' && method === 'GET') {
      return json(200, empty ? { ...OVERVIEW, counts_24h: { sent: 0, failed: 0, rejected: 0 }, series: OVERVIEW.series.map((h) => ({ ...h, sent: 0, failed: 0, rejected: 0 })), credentials: { total: 0, active: 0 } } : OVERVIEW);
    }
    if (path === '/upstream/test' && method === 'POST') return json(200, { ok: true, message: 'Connected to mail.example.com:587 in 118ms.', duration_ms: 118 });
    if (path === '/certificate/renew' && method === 'POST') return route.fulfill({ status: 202, contentType: 'application/json', body: '{}' });

    if (path === '/credentials' && method === 'GET') return json(200, { items: empty ? [] : CREDENTIALS });
    if (/^\/credentials\/\d+$/.test(path) && method === 'GET') {
      const id = Number(path.split('/').pop());
      const item = CREDENTIALS.find((c) => c.id === id);
      return item ? json(200, item) : json(404, { code: 'NOT_FOUND', message: 'Not found.' });
    }
    if (path === '/credentials' && method === 'POST') {
      return json(201, { credential: { ...must(CREDENTIALS[0], 'CREDENTIALS[0]'), id: 99, username: 'new-login' }, secret: 'sgw-fixture-secret-value' });
    }
    if (/^\/credentials\/\d+$/.test(path) && method === 'PATCH') return json(200, CREDENTIALS[0]);
    if (/^\/credentials\/\d+\/rotate$/.test(path) && method === 'POST') return json(200, { credential: CREDENTIALS[0], secret: 'sgw-fixture-rotated-secret' });
    if (/^\/credentials\/\d+$/.test(path) && method === 'DELETE') return route.fulfill({ status: 204, body: '' });

    if (path === '/messages' && method === 'GET') {
      if (empty) return json(200, { items: [], next_before: null });
      const status = url.searchParams.get('status');
      const credentialId = url.searchParams.get('credential_id');
      let filtered = MESSAGES;
      if (status) filtered = filtered.filter((m) => m.status === status);
      if (credentialId) filtered = filtered.filter((m) => String(m.credential_id) === credentialId);
      const before = url.searchParams.get('before');
      return json(200, page1(filtered, before ? Number(before) : undefined));
    }
    if (/^\/messages\/\d+$/.test(path) && method === 'GET') {
      const id = Number(path.split('/').pop());
      if (id === 5001) return json(200, MESSAGE_5001_DETAIL);
      const item = MESSAGES.find((m) => m.id === id);
      if (!item) return json(404, { code: 'NOT_FOUND', message: 'Not found.' });
      return json(200, { ...item, headers: [], headers_raw: '', headers_truncated: false });
    }

    if (path === '/audit' && method === 'GET') {
      if (empty) return json(200, { items: [], next_before: null });
      const action = url.searchParams.get('action');
      let filtered = AUDIT;
      if (action) filtered = filtered.filter((e) => e.action === action || (action.endsWith('.') && e.action.startsWith(action)));
      const before = url.searchParams.get('before');
      return json(200, page1(filtered, before ? Number(before) : undefined));
    }

    if (path === '/tokens' && method === 'GET') return json(200, { items: empty ? [] : TOKENS });
    if (path === '/tokens' && method === 'POST') return json(201, { token: { ...must(TOKENS[0], 'TOKENS[0]'), id: 99, name: 'new-token' }, secret: 'sgw-fixture-token-secret' });
    if (/^\/tokens\/\d+$/.test(path) && method === 'DELETE') return route.fulfill({ status: 204, body: '' });

    return json(404, { code: 'NOT_FOUND', message: 'Not found.' });
  });
}
