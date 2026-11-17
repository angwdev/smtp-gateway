import { useCallback, useMemo, useRef } from 'react';
import { useSearchParams } from 'react-router';

export interface MessageFilterParams {
  status?: string;
  credential_id?: string;
  q?: string;
  from?: string;
  to?: string;
}

const DEBOUNCE_MS = 400;

// Reads and writes a flat filter object to the query string. `q` is
// committed immediately on Enter, or after a short pause while typing.
export function useFilterParams<T extends Record<string, string | undefined>>() {
  const [searchParams, setSearchParams] = useSearchParams();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  const filter = useMemo(() => {
    const result = {} as T;
    for (const [key, value] of searchParams.entries()) {
      (result as Record<string, string>)[key] = value;
    }
    return result;
  }, [searchParams]);

  const apply = useCallback((next: T) => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(next)) {
      if (value) params.set(key, value);
    }
    setSearchParams(params, { replace: true });
  }, [setSearchParams]);

  const setFilter = useCallback((patch: Partial<T>, options?: { debounceQ?: boolean }) => {
    const next = { ...filter, ...patch };
    if (options?.debounceQ) {
      clearTimeout(timer.current);
      timer.current = setTimeout(() => apply(next), DEBOUNCE_MS);
      return;
    }
    clearTimeout(timer.current);
    apply(next);
  }, [filter, apply]);

  const clear = useCallback(() => {
    clearTimeout(timer.current);
    setSearchParams(new URLSearchParams(), { replace: true });
  }, [setSearchParams]);

  const activeCount = Object.values(filter).filter((v) => v !== undefined && v !== '').length;

  return { filter, setFilter, clear, activeCount };
}
