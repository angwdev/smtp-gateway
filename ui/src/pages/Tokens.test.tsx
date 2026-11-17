import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function token(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 2, name: 'grafana', hint: 'x9Qk', scope: 'read', created_at: '2026-10-01T00:00:00Z', created_by: 'ana@example.com',
    expires_at: null, last_used_at: null, last_used_ip: '', revoked_at: null, ...overrides,
  };
}

describe('Tokens', () => {
  it('creates a token, reveals the secret, and the curl example has no secret', async () => {
    const { calls } = renderApp('/tokens', {
      'GET /session': [200, SESSION],
      'GET /tokens': [200, { items: [] }],
      'POST /tokens': [201, { token: token({ id: 9, name: 'grafana' }), secret: 'sgw_abcdef' }],
    });
    await screen.findByText('No API tokens');
    await userEvent.click(screen.getByRole('button', { name: 'New token' }));
    await userEvent.type(screen.getByLabelText('Name'), 'grafana');
    await userEvent.click(screen.getByRole('button', { name: 'Create token' }));
    await screen.findByText('sgw_abcdef');
    const example = screen.getByText(/curl -H/);
    expect(example.textContent).not.toContain('sgw_abcdef');
    expect(example.textContent).toContain(window.location.origin);
    expect(calls.some((c) => c.method === 'POST' && (c.body as { name: string }).name === 'grafana')).toBe(true);
  });

  it('asks before revoking', async () => {
    renderApp('/tokens', {
      'GET /session': [200, SESSION],
      'GET /tokens': [200, { items: [token()] }],
      'DELETE /tokens/2': [204, null],
    });
    await userEvent.click(await screen.findByRole('button', { name: 'Revoke' }));
    await screen.findByRole('heading', { name: 'Revoke grafana?' });
    await userEvent.click(screen.getByRole('button', { name: 'Revoke token' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Revoke grafana?' })).not.toBeInTheDocument());
  });

  it('lists revoked tokens separately', async () => {
    renderApp('/tokens', {
      'GET /session': [200, SESSION],
      'GET /tokens': [200, { items: [token({ id: 3, name: 'old-script', revoked_at: '2026-11-01T00:00:00Z' })] }],
    });
    expect(await screen.findByText('Revoked')).toBeInTheDocument();
    expect(screen.getByText('old-script')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Revoke' })).not.toBeInTheDocument();
  });

  it('shows the person-only state for a 403', async () => {
    renderApp('/tokens', {
      'GET /session': [200, SESSION],
      'GET /tokens': [403, { code: 'FORBIDDEN', message: 'Tokens cannot manage tokens.' }],
    });
    expect(await screen.findByText('Sign in as a person to manage tokens')).toBeInTheDocument();
  });
});
