export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly field?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

type Query = Record<string, string | number | boolean | null | undefined>;

let onUnauthorized: () => void = () => undefined;

// The app shell registers what to do when the session is gone (go to /login).
export function setUnauthorizedHandler(handler: () => void): void {
  onUnauthorized = handler;
}

function withQuery(path: string, query?: Query): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== '') params.set(key, String(value));
  }
  const qs = params.toString();
  return qs ? `${path}?${qs}` : path;
}

async function request<T>(method: string, path: string, body?: unknown, query?: Query): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/api/v1${withQuery(path, query)}`, {
      method,
      credentials: 'same-origin',
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'NETWORK_ERROR', 'Could not reach the gateway. Check your connection and try again.');
  }
  if (res.status === 204) return undefined as T;

  const data: unknown = await res.json().catch(() => null);
  if (res.ok) return data as T;

  if (res.status === 401) onUnauthorized();
  const err = (data ?? {}) as { code?: string; message?: string; details?: { field?: string } };
  throw new ApiError(res.status, err.code ?? 'INTERNAL_ERROR', err.message ?? 'Something went wrong on the gateway.', err.details?.field);
}

export const api = {
  get: <T>(path: string, query?: Query) => request<T>('GET', path, undefined, query),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  delete: <T = void>(path: string) => request<T>('DELETE', path),
};
