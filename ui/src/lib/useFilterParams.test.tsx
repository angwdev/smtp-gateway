import { renderHook, act } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import { useFilterParams } from './useFilterParams';

function wrapper({ children }: { children: React.ReactNode }) {
  return <MemoryRouter initialEntries={['/messages?status=failed&credential_id=3']}>{children}</MemoryRouter>;
}

describe('useFilterParams', () => {
  it('reads the initial filter from the query string', () => {
    const { result } = renderHook(() => useFilterParams(), { wrapper });
    expect(result.current.filter).toEqual({ status: 'failed', credential_id: '3' });
    expect(result.current.activeCount).toBe(2);
  });

  it('clear removes every filter', () => {
    const { result } = renderHook(() => useFilterParams(), { wrapper });
    act(() => result.current.clear());
    expect(result.current.activeCount).toBe(0);
  });
});
