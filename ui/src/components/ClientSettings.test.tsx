import { render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { describe, expect, it } from 'vitest';

import { stubFetch } from '../test/fetch';
import { ClientSettings } from './ClientSettings';

describe('ClientSettings', () => {
  it('shows the server, ports, username and password note', async () => {
    stubFetch({
      'GET /overview': [200, {
        counts_24h: { sent: 0, failed: 0, rejected: 0 }, series: [], credentials: { total: 0, active: 0 },
        upstream: { addr: '', tls: 'none', username: '', envelope_from: '' },
        certificate: { source: '', label: '', names: [], not_after: null, days_left: 0, level: 'ok', acme: null },
        client_settings: { hostname: 'smtp.example.com', submission_port: '587', smtps_port: '465' }, version: '0.2.0',
      }],
    });
    const qc = new QueryClient();
    render(
      <QueryClientProvider client={qc}>
        <ClientSettings username="nextcloud" />
      </QueryClientProvider>,
    );
    expect(await screen.findByText('smtp.example.com')).toBeInTheDocument();
    expect(screen.getByText('nextcloud')).toBeInTheDocument();
    expect(screen.getByText('The one shown above')).toBeInTheDocument();
  });
});
