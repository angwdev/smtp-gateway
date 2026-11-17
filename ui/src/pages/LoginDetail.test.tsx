import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function credential() {
  return {
    id: 11, username: 'nextcloud', description: 'File sync', allowed_senders: [], rate_per_hour: 0, enabled: true,
    created_at: '2026-10-01T00:00:00Z', created_by: 'ana@example.com', updated_at: '2026-10-01T00:00:00Z',
    last_used_at: '2026-11-17T00:00:00Z', last_used_ip: '203.0.113.9', sent_count: 12,
  };
}

const EMPTY_MESSAGES = { items: [], next_before: null };

describe('LoginDetail', () => {
  it('asks before rotating, then reveals the new password', async () => {
    renderApp('/logins/11', {
      'GET /session': [200, SESSION],
      'GET /credentials/11': [200, credential()],
      'GET /messages': [200, EMPTY_MESSAGES],
      'POST /credentials/11/rotate': [200, { credential: credential(), secret: 'new-password-value' }],
    });
    await screen.findByRole('button', { name: 'Replace password' });
    await userEvent.click(screen.getByRole('button', { name: 'Replace password' }));
    await screen.findByRole('heading', { name: /Replace the password/ });
    await userEvent.click(screen.getByRole('button', { name: 'Replace password' }));
    await waitFor(() => expect(screen.getByText('new-password-value')).toBeInTheDocument());
  });

  it('asks before deleting, then navigates to the list', async () => {
    renderApp('/logins/11', {
      'GET /session': [200, SESSION],
      'GET /credentials/11': [200, credential()],
      'GET /credentials': [200, { items: [credential()] }],
      'GET /messages': [200, EMPTY_MESSAGES],
      'DELETE /credentials/11': [204, null],
    });
    await screen.findByRole('button', { name: 'Delete login' });
    await userEvent.click(screen.getByRole('button', { name: 'Delete login' }));
    await screen.findByRole('heading', { name: 'Delete nextcloud?' });
    await userEvent.click(screen.getByRole('button', { name: 'Delete login' }));
    await screen.findByRole('heading', { level: 1, name: 'Logins' });
  });

  it('shows a 404 state for a missing login', async () => {
    renderApp('/logins/999', {
      'GET /session': [200, SESSION],
      'GET /credentials/999': [404, { code: 'NOT_FOUND', message: 'Not found.' }],
    });
    expect(await screen.findByText('That login no longer exists')).toBeInTheDocument();
  });

  it('never renders the numeric id', async () => {
    renderApp('/logins/11', {
      'GET /session': [200, SESSION],
      'GET /credentials/11': [200, credential()],
      'GET /messages': [200, EMPTY_MESSAGES],
    });
    await screen.findByText('nextcloud');
    expect(document.body.textContent).not.toMatch(/\b11\b/);
  });

  it('sends only the changed fields on save', async () => {
    const { calls } = renderApp('/logins/11', {
      'GET /session': [200, SESSION],
      'GET /credentials/11': [200, credential()],
      'GET /messages': [200, EMPTY_MESSAGES],
      'PATCH /credentials/11': [200, { ...credential(), description: 'Updated' }],
    });
    await screen.findByLabelText('Description');
    await userEvent.clear(screen.getByLabelText('Description'));
    await userEvent.type(screen.getByLabelText('Description'), 'Updated');
    await userEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() => expect(screen.getByText('Saved.')).toBeInTheDocument());
    const patchCall = calls.find((c) => c.method === 'PATCH');
    expect(patchCall?.body).toEqual({ description: 'Updated' });
  });
});
