import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderApp } from '../test/render';

describe('AuthGate', () => {
  it('shows the shell once the session loads', async () => {
    renderApp('/', {
      'GET /session': [200, { auth: 'session', user: { email: 'a@b.c', name: 'Ana' }, scope: 'write', expires_at: null }],
      'GET /overview': [200, { counts_24h: { sent: 0, failed: 0, rejected: 0 }, series: [], credentials: { total: 0, active: 0 }, upstream: { addr: '', tls: 'none', username: '', envelope_from: '' }, certificate: { source: '', label: '', names: [], not_after: null, days_left: 0, level: 'ok', acme: null }, client_settings: { hostname: '', submission_port: '', smtps_port: '' }, version: '0.0.0' }],
    });
    await waitFor(() => expect(screen.getByRole('navigation', { name: 'Main' })).toBeInTheDocument());
  });

  it('sends a 401 to sign-in with the current location remembered', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign, pathname: '/messages', search: '?status=failed' });
    renderApp('/messages?status=failed', {
      'GET /session': [401, { code: 'UNAUTHORIZED', message: 'Sign in first.' }],
    });
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/login?next=%2Fmessages%3Fstatus%3Dfailed'));
  });

  it('shows a retry alert on a server error and refetches on click', async () => {
    renderApp('/', { 'GET /session': [500, { code: 'INTERNAL_ERROR', message: 'Database is locked.' }] });
    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument(), { timeout: 10_000 });
    expect(screen.getByText('Database is locked.')).toBeInTheDocument();
  }, 15_000);

  it('shows the signed-out error page and makes no session request', async () => {
    const calls = renderApp('/signed-out#error=Not%20allowed', {}).calls;
    expect(await screen.findByText('Sign-in did not work')).toBeInTheDocument();
    expect(screen.getByText('Not allowed')).toBeInTheDocument();
    expect(calls.some((c) => c.url.includes('/session'))).toBe(false);
  });
});
