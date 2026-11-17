import { vi } from 'vitest';

export interface Call { method: string; url: string; body: unknown }

// stubFetch replaces global fetch. `routes` maps "METHOD /path" (path without
// /api/v1, query string ignored) to [status, body]. It returns the calls made.
export function stubFetch(routes: Record<string, [number, unknown]>): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    calls.push({ method, url, body: init?.body ? JSON.parse(String(init.body)) : undefined });
    const path = url.replace(/^\/api\/v1/, '').replace(/\?.*$/, '');
    const route = routes[`${method} ${path}`];
    if (!route) return new Response(JSON.stringify({ code: 'NOT_FOUND', message: 'Not found.' }), { status: 404 });
    const [status, body] = route;
    return new Response(status === 204 ? null : JSON.stringify(body), { status });
  });
  return calls;
}
