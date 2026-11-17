import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';

import { stubFetch } from '../test/fetch';
import { useAudit } from './hooks';

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient();
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe('useAudit', () => {
  it('passes target_type through to the /audit query string', async () => {
    const calls = stubFetch({ 'GET /audit': [200, { items: [], next_before: null }] });
    const { result } = renderHook(() => useAudit({ target_type: 'credential' }), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.url).toContain('target_type=credential');
  });

  it('omits target_type from the query string when it is not set', async () => {
    const calls = stubFetch({ 'GET /audit': [200, { items: [], next_before: null }] });
    const { result } = renderHook(() => useAudit({}), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(calls[0]?.url).not.toContain('target_type');
  });
});
