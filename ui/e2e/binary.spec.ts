// Browser smoke test against the real built binary (not vite preview): the
// one thing no unit test proves, that the page renders under the real
// Content-Security-Policy, with the real per-request nonce, serving the
// files actually embedded in the binary. Runs only when SG_BINARY_URL is
// set (tools/smoke start sets up that server); otherwise every test skips.
import { expect, test } from '@playwright/test';

import { mockApi } from './fixtures';

const BINARY_URL = process.env.SG_BINARY_URL;

test.use({ baseURL: BINARY_URL });

test.beforeEach(() => {
  test.skip(!BINARY_URL, 'SG_BINARY_URL is not set; run tools/smoke start first');
});

const NONCE_RE = /<meta property="csp-nonce" nonce="([^"]+)">/;

function nonceFromCSP(csp: string | null | undefined): string | null {
  if (!csp) return null;
  const m = /'nonce-([^']+)'/.exec(csp);
  return m?.[1] ?? null;
}

test('the root document carries a real CSP nonce, not the build placeholder', async ({ request }) => {
  const res = await request.get('/');
  expect(res.status()).toBe(200);
  const csp = res.headers()['content-security-policy'];
  expect(csp).toContain("script-src 'self'");
  const headerNonce = nonceFromCSP(csp);
  expect(headerNonce).toBeTruthy();

  const body = await res.text();
  expect(body).not.toContain('__CSP_NONCE__');
  const htmlMatch = NONCE_RE.exec(body);
  expect(htmlMatch?.[1]).toBe(headerNonce);
});

test('two requests get two different nonces', async ({ request }) => {
  const [a, b] = await Promise.all([request.get('/'), request.get('/')]);
  const nonceA = nonceFromCSP(a.headers()['content-security-policy']);
  const nonceB = nonceFromCSP(b.headers()['content-security-policy']);
  expect(nonceA).toBeTruthy();
  expect(nonceB).toBeTruthy();
  expect(nonceA).not.toBe(nonceB);
});

test('/api/v1/nope is a JSON 404; /healthz is ok', async ({ request }) => {
  const nope = await request.get('/api/v1/nope');
  expect(nope.status()).toBe(404);
  expect(nope.headers()['content-type']).toContain('application/json');
  const body = await nope.json();
  expect(body.code).toBeTruthy();

  const health = await request.get('/healthz');
  expect(health.status()).toBe(200);
  expect((await health.text()).trim()).toBe('ok');
});

test('a hashed asset is served immutable, and gzipped files add content-encoding', async ({ request }) => {
  const root = await request.get('/');
  const html = await root.text();
  const scriptMatch = /<script[^>]+src="(\/assets\/[^"]+\.js)"/.exec(html);
  const assetPath = scriptMatch?.[1];
  expect(assetPath, `no script asset found in:\n${html}`).toBeTruthy();

  const plain = await request.get(assetPath as string);
  expect(plain.headers()['cache-control']).toBe('public, max-age=31536000, immutable');

  const gz = await request.get(assetPath as string, { headers: { 'accept-encoding': 'gzip' } });
  expect(gz.headers()['content-encoding']).toBe('gzip');
});

test('without a session, /messages bounces through /login to /signed-out', async ({ page }) => {
  const sessionCalls: number[] = [];
  page.on('response', (res) => {
    if (new URL(res.url()).pathname === '/api/v1/session') sessionCalls.push(res.status());
  });

  await page.goto('/messages');
  await expect(page).toHaveURL(/\/signed-out#error=/);
  await expect(page.getByRole('heading', { level: 1, name: 'Sign-in did not work' })).toBeVisible({ timeout: 15_000 });
  expect(sessionCalls).toContain(401);
});

const VIEWPORTS = [
  { name: 'phone', width: 390, height: 844 },
  { name: 'desktop', width: 1366, height: 900 },
] as const;
const THEMES = ['light', 'dark'] as const;

interface RouteCase { path: string; heading: RegExp | string }
const ROUTES: RouteCase[] = [
  { path: '/', heading: 'Overview' },
  { path: '/messages', heading: 'Messages' },
  { path: '/logins/11', heading: 'nextcloud' },
  { path: '/audit', heading: 'Audit' },
  { path: '/tokens', heading: 'API tokens' },
];

for (const viewport of VIEWPORTS) {
  for (const theme of THEMES) {
    test.describe(`${viewport.name} ${theme}`, () => {
      test.use({ viewport: { width: viewport.width, height: viewport.height } });

      for (const route of ROUTES) {
        test(`${route.path || 'overview'} renders under the real CSP`, async ({ page }) => {
          const cspMessages: string[] = [];
          page.on('console', (msg) => {
            const text = msg.text();
            if (text.includes('Content Security Policy') || text.includes('Refused to')) cspMessages.push(text);
          });

          await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
          await mockApi(page);
          await page.goto(route.path);
          await expect(page.getByRole('heading', { level: 1, name: route.heading })).toBeVisible({ timeout: 15_000 });

          expect(cspMessages, `CSP violations on ${route.path}: ${cspMessages.join('; ')}`).toEqual([]);

          await page.evaluate(() => document.fonts.ready);
          const fontLoaded = await page.evaluate(() => document.fonts.check('16px "Atkinson Hyperlegible Next Variable"'));
          expect(fontLoaded, `font not loaded on ${route.path}`).toBe(true);
        });
      }

      test('the primary button on /logins carries the accent background', async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        await page.goto('/logins');
        await expect(page.getByRole('heading', { level: 1, name: 'Logins' })).toBeVisible({ timeout: 15_000 });

        const button = page.getByRole('link', { name: 'New login' });
        await expect(button).toBeVisible();
        const bg = await button.evaluate((el) => getComputedStyle(el).backgroundColor);
        if (theme === 'light') expect(bg).toBe('rgb(194, 40, 35)');
        else expect(bg).not.toBe('rgba(0, 0, 0, 0)');
      });
    });
  }
}
