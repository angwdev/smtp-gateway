import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import type { AuditEvent } from '../api/types';
import { AuditRow } from './AuditRow';

function event(overrides: Partial<AuditEvent> = {}): AuditEvent {
  return {
    id: 41, created_at: '2026-11-17T00:00:00Z', actor: 'ana@example.com', actor_type: 'user',
    action: 'credential.update', target_type: 'credential', target_id: 3, target_label: 'nextcloud',
    result: 'ok', ip: '203.0.113.9', user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X) Chrome/120',
    request_id: 'req-fixture-0001', detail: '', changes: { rate_per_hour: { from: 0, to: 100 } },
    ...overrides,
  };
}

describe('AuditRow', () => {
  it('shows the sentence for a credential change', () => {
    render(<ul><AuditRow event={event()} /></ul>);
    expect(screen.getByText(/ana@example\.com/)).toBeInTheDocument();
    expect(screen.getByText(/changed login nextcloud/)).toBeInTheDocument();
  });

  it('expands to show address, browser and the change table', async () => {
    render(<ul><AuditRow event={event()} /></ul>);
    await userEvent.click(screen.getByRole('button'));
    expect(screen.getByText('203.0.113.9')).toBeInTheDocument();
    expect(screen.getByText(/Chrome on macOS/)).toBeInTheDocument();
    expect(screen.getByText('Hourly limit')).toBeInTheDocument();
    expect(screen.getByText('No limit')).toBeInTheDocument();
    expect(screen.getByText('100 per hour')).toBeInTheDocument();
  });

  it('shows method and path for an api.call event', () => {
    render(<ul><AuditRow event={event({ actor: 'grafana', actor_type: 'token', action: 'api.call', detail: 'GET /messages', changes: {} })} /></ul>);
    expect(screen.getByText('GET /messages')).toBeInTheDocument();
  });

  it('carries the result word for screen readers on denied and error rows', () => {
    render(<ul><AuditRow event={event({ result: 'denied', action: 'session.login_denied' })} /></ul>);
    expect(screen.getByText('Refused')).toBeInTheDocument();
  });

  it('never renders the request id', () => {
    render(<ul><AuditRow event={event()} /></ul>);
    expect(document.body.textContent).not.toContain('req-fixture-0001');
  });
});
