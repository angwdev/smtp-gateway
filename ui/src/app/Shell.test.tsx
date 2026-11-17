import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };
const OVERVIEW = {
  counts_24h: { sent: 0, failed: 0, rejected: 0 },
  series: [],
  credentials: { total: 0, active: 0 },
  upstream: { addr: '', tls: 'none', username: '', envelope_from: '' },
  certificate: { source: '', label: '', names: [], not_after: null, days_left: 0, level: 'ok', acme: null },
  client_settings: { hostname: '', submission_port: '', smtps_port: '' },
  version: '0.0.0',
};

describe('Shell', () => {
  it('renders both navigations with the expected links', async () => {
    renderApp('/', { 'GET /session': [200, SESSION], 'GET /overview': [200, OVERVIEW] });
    await waitFor(() => expect(document.querySelectorAll('nav[aria-label="Main"]')).toHaveLength(2));
    const navs = document.querySelectorAll('nav[aria-label="Main"]');
    for (const nav of navs) {
      expect(nav.querySelector('a[href="/logins"]')).not.toBeNull();
      expect(nav.querySelector('a[href="/messages"]')).not.toBeNull();
      expect(nav.querySelector('a[href="/audit"]')).not.toBeNull();
    }
  });

  it('marks Messages current on a detail route and Overview only on /', async () => {
    renderApp('/messages/9', { 'GET /session': [200, SESSION], 'GET /overview': [200, OVERVIEW] });
    await waitFor(() => expect(screen.getAllByText('Messages').length).toBeGreaterThan(0));
    const messagesLinks = screen.getAllByText('Messages').map((el) => el.closest('a'));
    for (const link of messagesLinks) {
      expect(link).toHaveAttribute('aria-current', 'page');
    }
    const overviewLinks = screen.getAllByText('Overview').map((el) => el.closest('a'));
    for (const link of overviewLinks) {
      expect(link).not.toHaveAttribute('aria-current');
    }
  });

  it('marks the More tab current on /tokens', async () => {
    renderApp('/tokens', { 'GET /session': [200, SESSION], 'GET /overview': [200, OVERVIEW] });
    await waitFor(() => expect(screen.getAllByText('More').length).toBeGreaterThan(0));
    const moreTab = screen.getAllByText('More').map((el) => el.closest('a')).find((a) => a?.getAttribute('href') === '/more');
    expect(moreTab).toHaveAttribute('aria-current', 'page');
  });

  it('signs out and follows the redirect', async () => {
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign });
    const { calls } = renderApp('/', {
      'GET /session': [200, SESSION],
      'GET /overview': [200, OVERVIEW],
      'DELETE /session': [200, { redirect_url: '/signed-out' }],
    });
    const button = await screen.findByRole('button', { name: 'Sign out' });
    button.click();
    await waitFor(() => expect(assign).toHaveBeenCalledWith('/signed-out'));
    expect(calls.some((c) => c.method === 'DELETE' && c.url.includes('/session'))).toBe(true);
  });
});
