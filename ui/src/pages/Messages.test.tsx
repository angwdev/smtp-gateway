import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function message(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 5001, created_at: '2026-11-16T08:00:00Z', credential_id: 11, username: 'nextcloud',
    remote_ip: '203.0.113.9', mail_from: 'a@b.c', rcpt_to: ['d@e.f'], size: 4096,
    status: 'sent', detail: '2.0.0 Ok', subject: 'Weekly report', ...overrides,
  };
}

const CREDENTIALS = { items: [{ id: 11, username: 'nextcloud', description: '', allowed_senders: [], rate_per_hour: 0, enabled: true, created_at: '2026-10-01T00:00:00Z', created_by: 'ana@example.com', updated_at: '2026-10-01T00:00:00Z', last_used_at: null, last_used_ip: '', sent_count: 0 }] };

describe('Messages', () => {
  it('shows the empty state with no filters', async () => {
    renderApp('/messages', {
      'GET /session': [200, SESSION],
      'GET /credentials': [200, CREDENTIALS],
      'GET /messages': [200, { items: [], next_before: null }],
    });
    expect(await screen.findByText('No messages yet')).toBeInTheDocument();
  });

  it('shows a failed row with its detail', async () => {
    renderApp('/messages', {
      'GET /session': [200, SESSION],
      'GET /credentials': [200, CREDENTIALS],
      'GET /messages': [200, { items: [message({ status: 'failed', detail: 'Upstream refused the message: 550 blocked' })], next_before: null }],
    });
    await screen.findAllByText('Weekly report');
    expect(screen.getAllByText('Upstream refused the message: 550 blocked').length).toBeGreaterThan(0);
  });

  it('shows (no subject) for a message with an empty subject', async () => {
    renderApp('/messages', {
      'GET /session': [200, SESSION],
      'GET /credentials': [200, CREDENTIALS],
      'GET /messages': [200, { items: [message({ subject: '' })], next_before: null }],
    });
    await waitFor(() => expect(screen.getAllByText('(no subject)').length).toBeGreaterThan(0));
  });

  it('requests the next page with the before cursor on Show older', async () => {
    const { calls } = renderApp('/messages', {
      'GET /session': [200, SESSION],
      'GET /credentials': [200, CREDENTIALS],
      'GET /messages': [200, { items: [message()], next_before: 4000 }],
    });
    const button = await screen.findByRole('button', { name: 'Show older' });
    await userEvent.click(button);
    await waitFor(() => expect(calls.some((c) => c.url.includes('before=4000'))).toBe(true));
  });
});
