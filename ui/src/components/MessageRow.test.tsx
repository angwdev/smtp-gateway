import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import type { Message } from '../api/types';
import { MessageRow } from './MessageRow';

function message(overrides: Partial<Message> = {}): Message {
  return {
    id: 5001, created_at: '2026-11-16T08:00:00Z', credential_id: 11, username: 'nextcloud',
    remote_ip: '203.0.113.9', mail_from: 'a@b.c', rcpt_to: ['d@e.f'], size: 4096,
    status: 'sent', detail: '2.0.0 Ok', subject: 'Weekly report', ...overrides,
  };
}

describe('MessageRow', () => {
  it('links to the message detail', () => {
    render(<MemoryRouter><ul><MessageRow message={message()} /></ul></MemoryRouter>);
    expect(screen.getAllByRole('link')[0]).toHaveAttribute('href', '/messages/5001');
  });

  it('shows (no subject) for an empty subject', () => {
    render(<MemoryRouter><ul><MessageRow message={message({ subject: '' })} /></ul></MemoryRouter>);
    expect(screen.getAllByText('(no subject)').length).toBeGreaterThan(0);
  });
});
