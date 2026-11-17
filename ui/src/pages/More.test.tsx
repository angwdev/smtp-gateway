import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };
const OVERVIEW = {
  counts_24h: { sent: 0, failed: 0, rejected: 0 }, series: [], credentials: { total: 0, active: 0 },
  upstream: { addr: '', tls: 'none', username: '', envelope_from: '' },
  certificate: { source: '', label: '', names: [], not_after: null, days_left: 0, level: 'ok', acme: null },
  client_settings: { hostname: '', submission_port: '', smtps_port: '' }, version: '0.2.0',
};

describe('More', () => {
  it('shows the signed-in name and signs out', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign });
    const { calls } = renderApp('/more', {
      'GET /session': [200, SESSION],
      'GET /overview': [200, OVERVIEW],
      'DELETE /session': [200, { redirect_url: '/signed-out' }],
    });
    expect(await screen.findByText(/Ana Gomez/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(calls.some((c) => c.method === 'DELETE' && c.url.includes('/session'))).toBe(true);
  });
});
