import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };
const OVERVIEW = {
  counts_24h: { sent: 0, failed: 0, rejected: 0 }, series: [], credentials: { total: 0, active: 0 },
  upstream: { addr: '', tls: 'none', username: '', envelope_from: '' },
  certificate: { source: '', label: '', names: [], not_after: null, days_left: 0, level: 'ok', acme: null },
  client_settings: { hostname: 'smtp.example.com', submission_port: '587', smtps_port: '465' }, version: '0.2.0',
};

describe('LoginNew', () => {
  it('creates a login, shows the secret once, and Done navigates to the detail page', async () => {
    const { calls } = renderApp('/logins/new', {
      'GET /session': [200, SESSION],
      'GET /overview': [200, OVERVIEW],
      'POST /credentials': [201, {
        credential: { id: 42, username: 'grafana', description: '', allowed_senders: [], rate_per_hour: 0, enabled: true, created_at: '2026-11-17T00:00:00Z', created_by: 'ana@example.com', updated_at: '2026-11-17T00:00:00Z', last_used_at: null, last_used_ip: '', sent_count: 0 },
        secret: 's3cret-password',
      }],
    });
    await screen.findByLabelText('Username');
    await userEvent.type(screen.getByLabelText('Username'), 'grafana');
    await userEvent.click(screen.getByRole('button', { name: 'Create login' }));
    await screen.findByText('s3cret-password');
    expect(calls.some((c) => c.method === 'POST' && c.url.includes('/credentials') && (c.body as { username: string }).username === 'grafana')).toBe(true);

    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign });
    await userEvent.click(screen.getByRole('button', { name: 'Done' }));
    expect(assign).toHaveBeenCalledWith('/logins/42');
  });
});
