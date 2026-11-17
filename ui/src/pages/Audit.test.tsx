import { screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function event(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: 41, created_at: '2026-11-17T00:00:00Z', actor: 'ana@example.com', actor_type: 'user',
    action: 'credential.update', target_type: 'credential', target_id: 3, target_label: 'nextcloud',
    result: 'ok', ip: '203.0.113.9', user_agent: 'Mozilla/5.0', request_id: 'req-fixture-0001',
    detail: '', changes: {}, ...overrides,
  };
}

describe('Audit', () => {
  it('shows empty state without filters', async () => {
    renderApp('/audit', { 'GET /session': [200, SESSION], 'GET /audit': [200, { items: [], next_before: null }] });
    expect(await screen.findByText('Nothing recorded yet')).toBeInTheDocument();
  });

  it('sends a group filter as a dotted prefix', async () => {
    const { calls } = renderApp('/audit?action=credential.', {
      'GET /session': [200, SESSION],
      'GET /audit': [200, { items: [event()], next_before: null }],
    });
    await waitFor(() => expect(calls.some((c) => c.url.includes('action=credential.'))).toBe(true));
  });

  it('round-trips a filter through the URL', async () => {
    renderApp('/audit', { 'GET /session': [200, SESSION], 'GET /audit': [200, { items: [event()], next_before: null }] });
    await screen.findByText(/changed login/);
    expect(window.location.search).toBe('');
  });
});
