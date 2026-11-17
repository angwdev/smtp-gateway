import { screen, waitFor } from '@testing-library/react';
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

describe('MessageDetail', () => {
  it('shows parsed headers in order and toggles the raw block', async () => {
    renderApp('/messages/5001', {
      'GET /session': [200, SESSION],
      'GET /messages/5001': [200, {
        ...message(),
        headers: [{ name: 'From', value: 'A <a@b.c>' }, { name: 'Subject', value: 'Weekly report' }],
        headers_raw: 'From: A <a@b.c>\r\nSubject: Weekly report\r\n',
        headers_truncated: true,
      }],
    });
    const headersRegion = await screen.findByRole('region', { name: 'Headers' });
    const names = Array.from(headersRegion.querySelectorAll('dt')).map((el) => el.textContent);
    expect(names).toEqual(['From', 'Subject']);
    expect(screen.getByText('Only the first 16 KB of the headers are stored.')).toBeInTheDocument();
    expect(screen.queryByText(/From: A/)).not.toBeInTheDocument();
    screen.getByRole('button', { name: 'Show raw headers' }).click();
    await waitFor(() => expect(screen.getByText(/From: A/)).toBeInTheDocument());
  });

  it('shows the note when headers were not stored', async () => {
    renderApp('/messages/5001', {
      'GET /session': [200, SESSION],
      'GET /messages/5001': [200, { ...message(), headers: [], headers_raw: '', headers_truncated: false }],
    });
    expect(await screen.findByText('Headers were not stored for this message.')).toBeInTheDocument();
  });

  it('shows a 404 state', async () => {
    renderApp('/messages/9999', {
      'GET /session': [200, SESSION],
      'GET /messages/9999': [404, { code: 'NOT_FOUND', message: 'Not found.' }],
    });
    expect(await screen.findByText('That message is no longer in the log')).toBeInTheDocument();
  });

  it('never renders the numeric id', async () => {
    renderApp('/messages/5001', {
      'GET /session': [200, SESSION],
      'GET /messages/5001': [200, { ...message(), headers: [], headers_raw: '', headers_truncated: false }],
    });
    await screen.findByText('Weekly report');
    expect(document.body.textContent).not.toMatch(/\b5001\b/);
  });
});
