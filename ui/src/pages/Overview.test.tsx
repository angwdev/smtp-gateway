import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';

import { renderApp } from '../test/render';

const SESSION = { auth: 'session', user: { email: 'ana@example.com', name: 'Ana Gomez' }, scope: 'write', expires_at: null };

function overview(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    counts_24h: { sent: 40, failed: 0, rejected: 1 },
    series: Array.from({ length: 24 }, (_, i) => ({ hour: `2026-11-16T${String(i).padStart(2, '0')}:00:00Z`, sent: i === 8 ? 10 : 1, failed: 0, rejected: 0 })),
    credentials: { total: 5, active: 5 },
    upstream: { addr: 'mail.example.com:587', tls: 'starttls', username: 'relay@example.com', envelope_from: '' },
    certificate: { source: 'acme', label: "Let's Encrypt via Cloudflare DNS", names: ['smtp.example.com'], not_after: '2027-01-01T00:00:00Z', days_left: 61, level: 'ok', acme: { renewing: false, ari: true, next_check: '2026-11-18T00:00:00Z', last_error: '' } },
    client_settings: { hostname: 'smtp.example.com', submission_port: '587', smtps_port: '465' },
    version: '0.2.0',
    ...overrides,
  };
}

describe('Overview', () => {
  it('links the counts to the filtered log and leaves zero counts uncoloured', async () => {
    renderApp('/', { 'GET /session': [200, SESSION], 'GET /overview': [200, overview()] });
    const sentLink = await screen.findByRole('link', { name: '40 sent in the last 24 hours, show them' });
    expect(sentLink).toHaveAttribute('href', '/messages?status=sent');
    const failedLink = screen.getByRole('link', { name: '0 failed in the last 24 hours, show them' });
    const failedCount = failedLink.querySelector('span');
    expect(failedCount?.className).not.toContain('text-failed');
  });

  it('tests the upstream connection and shows the result', async () => {
    renderApp('/', {
      'GET /session': [200, SESSION],
      'GET /overview': [200, overview()],
      'POST /upstream/test': [200, { ok: true, message: 'Connected in 120ms.', duration_ms: 120 }],
    });
    const button = await screen.findByRole('button', { name: 'Test connection' });
    await userEvent.click(button);
    await waitFor(() => expect(screen.getByText('Connected in 120ms.')).toBeInTheDocument());
  });

  it('hides the ACME rows and Renew now when acme is null', async () => {
    renderApp('/', { 'GET /session': [200, SESSION], 'GET /overview': [200, overview({ certificate: { ...overview().certificate, acme: null } })] });
    await screen.findByText("Let's Encrypt via Cloudflare DNS");
    expect(screen.queryByRole('button', { name: 'Renew now' })).not.toBeInTheDocument();
    expect(screen.queryByText('Automatic renewal')).not.toBeInTheDocument();
  });

  it('hides the port rows when they are empty', async () => {
    renderApp('/', { 'GET /session': [200, SESSION], 'GET /overview': [200, overview({ client_settings: { hostname: 'smtp.example.com', submission_port: '', smtps_port: '' } })] });
    await screen.findByText('Settings for mail clients');
    expect(screen.queryByText('Port with STARTTLS')).not.toBeInTheDocument();
    expect(screen.queryByText('Port with TLS')).not.toBeInTheDocument();
  });

  it('shows the empty logins state when there are none', async () => {
    renderApp('/', { 'GET /session': [200, SESSION], 'GET /overview': [200, overview({ credentials: { total: 0, active: 0 } })] });
    expect(await screen.findByText('Create the first login')).toBeInTheDocument();
  });
});
