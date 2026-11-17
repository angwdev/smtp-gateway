import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function credential(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 11, username: 'nextcloud', description: 'File sync', allowed_senders: [], rate_per_hour: 0, enabled: true,
    created_at: '2026-10-01T00:00:00Z', created_by: 'ana@example.com', updated_at: '2026-10-01T00:00:00Z',
    last_used_at: '2026-11-17T00:00:00Z', last_used_ip: '203.0.113.9', sent_count: 12, ...overrides,
  };
}

describe('Logins list', () => {
  it('sorts turned-on logins first, then by username, and marks turned-off logins', async () => {
    renderApp('/logins', {
      'GET /session': [200, SESSION],
      'GET /credentials': [200, { items: [
        credential({ id: 11, username: 'zeta', enabled: true }),
        credential({ id: 12, username: 'alpha', enabled: false }),
        credential({ id: 13, username: 'beta', enabled: true }),
      ] }],
    });
    await screen.findByText('zeta');
    const rows = screen.getAllByRole('link').filter((a) => /^\/logins\/\d+$/.test(a.getAttribute('href') ?? ''));
    const order = rows.map((r) => r.textContent ?? '');
    expect(order[0]).toContain('beta');
    expect(order[1]).toContain('zeta');
    expect(order[2]).toContain('Offalpha');
  });

  it('shows the search field only above 8 logins', async () => {
    const few = Array.from({ length: 3 }, (_, i) => credential({ id: 20 + i, username: `app${i}` }));
    renderApp('/logins', { 'GET /session': [200, SESSION], 'GET /credentials': [200, { items: few }] });
    await screen.findByText('app0');
    expect(screen.queryByLabelText('Find a login')).not.toBeInTheDocument();
  });

  it('shows the search field and filters when there are more than 8', async () => {
    const many = Array.from({ length: 9 }, (_, i) => credential({ id: 30 + i, username: `app${i}` }));
    renderApp('/logins', { 'GET /session': [200, SESSION], 'GET /credentials': [200, { items: many }] });
    await screen.findByText('app0');
    const search = screen.getByLabelText('Find a login');
    await userEvent.type(search, 'app3');
    await waitFor(() => expect(screen.queryByText('app0')).not.toBeInTheDocument());
    expect(screen.getByText('app3')).toBeInTheDocument();
  });

  it('shows the empty state with no logins', async () => {
    renderApp('/logins', { 'GET /session': [200, SESSION], 'GET /credentials': [200, { items: [] }] });
    expect(await screen.findByText('Create the first login')).toBeInTheDocument();
  });
});
