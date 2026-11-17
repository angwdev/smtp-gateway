import { stubFetch } from '../test/fetch';
import { api, ApiError, setUnauthorizedHandler } from './client';

describe('api client', () => {
  it('GETs JSON under /api/v1 with same-origin credentials', async () => {
    const calls = stubFetch({ 'GET /credentials': [200, { items: [] }] });
    await expect(api.get('/credentials')).resolves.toEqual({ items: [] });
    expect(calls[0]?.url).toBe('/api/v1/credentials');
  });

  it('builds a query string and skips empty values', async () => {
    const calls = stubFetch({ 'GET /messages': [200, { items: [], next_before: null }] });
    await api.get('/messages', { status: 'failed', q: '', credential_id: undefined, limit: 50 });
    expect(calls[0]?.url).toBe('/api/v1/messages?status=failed&limit=50');
  });

  it('sends a JSON body', async () => {
    const calls = stubFetch({ 'POST /tokens': [201, { token: {}, secret: 'sgw_x' }] });
    await api.post('/tokens', { name: 'grafana', scope: 'read', expires_at: null });
    expect(calls[0]?.body).toEqual({ name: 'grafana', scope: 'read', expires_at: null });
  });

  it('returns undefined for 204', async () => {
    stubFetch({ 'DELETE /credentials/3': [204, null] });
    await expect(api.delete('/credentials/3')).resolves.toBeUndefined();
  });

  it('throws ApiError with the code, message and field', async () => {
    stubFetch({ 'POST /credentials': [400, { code: 'VALIDATION_ERROR', message: 'Use letters.', details: { field: 'username' } }] });
    const err = await api.post('/credentials', {}).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 400, code: 'VALIDATION_ERROR', message: 'Use letters.', field: 'username' });
  });

  it('calls the unauthorized handler on 401', async () => {
    stubFetch({ 'GET /overview': [401, { code: 'UNAUTHORIZED', message: 'Sign in.' }] });
    const handler = vi.fn();
    setUnauthorizedHandler(handler);
    await api.get('/overview').catch(() => undefined);
    expect(handler).toHaveBeenCalledOnce();
  });

  it('turns a network failure into an ApiError the UI can show', async () => {
    vi.stubGlobal('fetch', async () => { throw new TypeError('Failed to fetch'); });
    const err = await api.get('/overview').catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 0, code: 'NETWORK_ERROR' });
  });
});
