import { expect, test } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

import { mockApi } from './fixtures';

interface Viewport { name: string; width: number; height: number }
const VIEWPORTS: Viewport[] = [
  { name: 'phone', width: 390, height: 844 },
  { name: 'phone-small', width: 360, height: 740 },
  { name: 'tablet', width: 820, height: 1180 },
  { name: 'tablet-landscape', width: 1180, height: 820 },
  { name: 'desktop', width: 1366, height: 900 },
];
const THEMES: ('light' | 'dark')[] = ['light', 'dark'];

interface RouteCase { name: string; path: string; heading: RegExp | string; empty?: boolean }

const DATA_ROUTES: RouteCase[] = [
  { name: 'overview', path: '/', heading: 'Overview' },
  { name: 'logins', path: '/logins', heading: 'Logins' },
  { name: 'logins-new', path: '/logins/new', heading: 'New login' },
  { name: 'logins-detail', path: '/logins/11', heading: 'nextcloud' },
  { name: 'messages', path: '/messages', heading: 'Messages' },
  { name: 'messages-failed', path: '/messages?status=failed', heading: 'Messages' },
  { name: 'message-detail', path: '/messages/5001', heading: /Sent/ },
  { name: 'audit', path: '/audit', heading: 'Audit' },
  { name: 'tokens', path: '/tokens', heading: 'API tokens' },
  { name: 'more', path: '/more', heading: 'More' },
  { name: 'signed-out', path: '/signed-out', heading: "You're signed out" },
  { name: 'signed-out-error', path: '/signed-out#error=Your%20account%20is%20not%20allowed', heading: 'Sign-in did not work' },
];

const EMPTY_ROUTES: RouteCase[] = [
  { name: 'overview-empty', path: '/', heading: 'Overview', empty: true },
  { name: 'logins-empty', path: '/logins', heading: 'Logins', empty: true },
  { name: 'messages-empty', path: '/messages', heading: 'Messages', empty: true },
  { name: 'audit-empty', path: '/audit', heading: 'Audit', empty: true },
  { name: 'tokens-empty', path: '/tokens', heading: 'API tokens', empty: true },
];

async function checkPage(page: import('@playwright/test').Page, label: string, viewport: Viewport) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
  expect(overflow, `${label}: horizontal overflow`).toBe(true);

  if (viewport.width < 1024) {
    const boxes = await page.evaluate(() => {
      const selector = 'a[href], button, [role="tab"], input:not([type="hidden"]), [role="combobox"], [role="switch"]';
      const results: { tag: string; w: number; h: number }[] = [];
      document.querySelectorAll(selector).forEach((el) => {
        const style = window.getComputedStyle(el);
        if (style.display === 'inline') return;
        if (style.visibility === 'hidden' || style.display === 'none') return;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 && rect.height === 0) return;
        // sr-only elements (e.g. the skip link) collapse to a 1x1 box until
        // focused; they are not a visible tap target in that state.
        if (rect.width <= 1 && rect.height <= 1) return;
        results.push({ tag: el.outerHTML.slice(0, 80), w: rect.width, h: rect.height });
      });
      return results;
    });
    for (const box of boxes) {
      expect(box.w >= 44 && box.h >= 44, `${label}: touch target too small ${box.w}x${box.h} (${box.tag})`).toBe(true);
    }
  }

  const bodyText = await page.evaluate(() => document.body.textContent ?? '');
  expect(bodyText, `${label}: leaked request_id`).not.toMatch(/req-fixture-/);
  if (label.includes('logins-detail')) expect(bodyText, `${label}: leaked id 11`).not.toMatch(/\b11\b/);
  if (label.includes('message-detail')) expect(bodyText, `${label}: leaked id 5001`).not.toMatch(/\b5001\b/);

  const h1Count = await page.evaluate(() => document.querySelectorAll('h1').length);
  expect(h1Count, `${label}: expected exactly one h1`).toBe(1);
  const title = await page.title();
  expect(title.endsWith('· SMTP Gateway'), `${label}: document.title "${title}"`).toBe(true);

  const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
  expect(axe.violations, `${label}: axe violations ${JSON.stringify(axe.violations.map((v) => v.id))}`).toEqual([]);
}

for (const viewport of VIEWPORTS) {
  for (const theme of THEMES) {
    test.describe(`${viewport.name} ${theme}`, () => {
      test.use({ viewport: { width: viewport.width, height: viewport.height } });

      for (const route of [...DATA_ROUTES, ...EMPTY_ROUTES]) {
        test(`${route.name}`, async ({ page }) => {
          const consoleErrors: string[] = [];
          const failedRequests: string[] = [];
          page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text()); });
          page.on('requestfailed', (req) => failedRequests.push(req.url()));

          await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
          await mockApi(page, { empty: route.empty });
          await page.goto(route.path);
          await expect(page.getByRole('heading', { level: 1, name: route.heading })).toBeVisible({ timeout: 10_000 });

          const label = `${route.name}-${viewport.name}-${theme}`;
          await checkPage(page, label, viewport);

          expect(consoleErrors, `${label}: console errors`).toEqual([]);
          expect(failedRequests, `${label}: failed requests`).toEqual([]);

          await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
        });
      }

      if (viewport.name === 'phone' || viewport.name === 'phone-small') {
        test('messages-filter-open', async ({ page }) => {
          await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
          await mockApi(page);
          await page.goto('/messages');
          await expect(page.getByRole('heading', { level: 1, name: 'Messages' })).toBeVisible({ timeout: 10_000 });
          await page.getByRole('button', { name: /^Filter/ }).click();
          await expect(page.getByRole('heading', { name: 'Filter' })).toBeVisible();
          await page.screenshot({ path: `shots-out/messages-filter-open-${viewport.name}-${theme}.png`, fullPage: true });
        });
      }
    });
  }
}

test.describe('audit expanded detail', () => {
  for (const viewport of VIEWPORTS) {
    for (const theme of THEMES) {
      test(`audit-open-${viewport.name}-${theme}`, async ({ page }) => {
        await page.setViewportSize({ width: viewport.width, height: viewport.height });
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        await page.goto('/audit');
        await expect(page.getByRole('heading', { level: 1, name: 'Audit' })).toBeVisible({ timeout: 10_000 });

        // Open rows one at a time until the one with a change table appears.
        const buttons = page.locator('li > button');
        const count = await buttons.count();
        let opened = false;
        for (let i = 0; i < count; i += 1) {
          await buttons.nth(i).click();
          if (await page.getByText('What changed').isVisible().catch(() => false)) { opened = true; break; }
          await buttons.nth(i).click();
        }
        expect(opened, 'expected one audit row with a change table').toBe(true);

        const label = `audit-open-${viewport.name}-${theme}`;
        await checkPage(page, label, viewport);
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
      });
    }
  }
});

// States that only appear after an interaction, not on a plain route load:
// a saved-settings notice, both secret-reveal flows, a non-field form error
// and a page-level failure (no result colour, no card, errors stay on page).
// Shot at phone (390) and desktop (1366), both themes, per the plan's rule
// for interaction-gated states (see "messages-filter-open" above).
const INTERACTION_VIEWPORTS = VIEWPORTS.filter((v) => v.name === 'phone' || v.name === 'desktop');

for (const viewport of INTERACTION_VIEWPORTS) {
  for (const theme of THEMES) {
    test.describe(`notice and error states ${viewport.name} ${theme}`, () => {
      test.use({ viewport: { width: viewport.width, height: viewport.height } });

      test(`login-saved-${viewport.name}-${theme}`, async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        await page.goto('/logins/11');
        await expect(page.getByRole('heading', { level: 1, name: 'nextcloud' })).toBeVisible({ timeout: 10_000 });
        await page.getByLabel('Description').fill('File sync (updated)');
        await page.getByRole('button', { name: 'Save changes' }).click();
        await expect(page.getByText('Saved.')).toBeVisible();

        const label = `login-saved-${viewport.name}-${theme}`;
        await checkPage(page, label, viewport);
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
      });

      test(`secret-reveal-new-${viewport.name}-${theme}`, async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        await page.goto('/logins/new');
        await expect(page.getByRole('heading', { level: 1, name: 'New login' })).toBeVisible({ timeout: 10_000 });
        await page.getByLabel('Username').fill('new-app');
        await page.getByRole('button', { name: 'Create login' }).click();
        await expect(page.getByRole('heading', { name: /Password for/ })).toBeVisible();

        const label = `secret-reveal-new-${viewport.name}-${theme}`;
        await checkPage(page, label, viewport);
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
      });

      test(`secret-reveal-rotate-${viewport.name}-${theme}`, async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        await page.goto('/logins/11');
        await expect(page.getByRole('heading', { level: 1, name: 'nextcloud' })).toBeVisible({ timeout: 10_000 });
        await page.getByRole('button', { name: 'Replace password' }).click();
        await expect(page.getByRole('heading', { name: /Replace the password/ })).toBeVisible();
        await page.getByRole('button', { name: 'Replace password' }).click();
        await expect(page.getByText('sgw-fixture-rotated-secret')).toBeVisible();

        const label = `secret-reveal-rotate-${viewport.name}-${theme}`;
        await checkPage(page, label, viewport);
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
      });

      test(`form-validation-error-${viewport.name}-${theme}`, async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        // A non-field error (no `details.field`): the form shows the
        // shared Notice, not a per-field helper text, and it must not be
        // the "failed" result colour (it is not a delivery result).
        await page.route('**/api/v1/credentials', async (route) => {
          if (route.request().method() !== 'POST') return route.fallback();
          return route.fulfill({
            status: 409,
            contentType: 'application/json',
            body: JSON.stringify({ code: 'CONFLICT', message: 'Too many logins were created in the last minute. Wait a moment and try again.' }),
          });
        });
        await page.goto('/logins/new');
        await expect(page.getByRole('heading', { level: 1, name: 'New login' })).toBeVisible({ timeout: 10_000 });
        await page.getByLabel('Username').fill('new-app');
        await page.getByRole('button', { name: 'Create login' }).click();
        await expect(page.getByText(/Too many logins were created/)).toBeVisible();

        const label = `form-validation-error-${viewport.name}-${theme}`;
        await checkPage(page, label, viewport);
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });
      });

      test(`page-error-state-${viewport.name}-${theme}`, async ({ page }) => {
        await page.addInitScript((mode) => localStorage.setItem('mui-mode', mode), theme);
        await mockApi(page);
        // A full list-load failure. The page intentionally renders no
        // PageHeader/h1 in this state (DESIGN.md's Error state is "what
        // failed + Try again", not a full page chrome), so this test runs
        // its own light checks instead of the shared checkPage helper.
        await page.route('**/api/v1/credentials', async (route) => {
          if (route.request().method() !== 'GET') return route.fallback();
          return route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: JSON.stringify({ code: 'INTERNAL_ERROR', message: 'The database is temporarily unavailable. Try again in a moment.' }),
          });
        });
        await page.goto('/logins');
        await expect(page.getByRole('alert')).toBeVisible({ timeout: 10_000 });
        await expect(page.getByText('Could not load this.')).toBeVisible();
        await expect(page.getByRole('button', { name: 'Try again' })).toBeVisible();

        const overflow = await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);
        expect(overflow, 'page-error-state: horizontal overflow').toBe(true);

        const label = `page-error-state-${viewport.name}-${theme}`;
        // Shot before the axe scan: axe can move focus while auditing tab
        // order, which would otherwise leave a stray focus ring in the shot.
        await page.screenshot({ path: `shots-out/${label}.png`, fullPage: true });

        const axe = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
        expect(axe.violations, `page-error-state: axe violations ${JSON.stringify(axe.violations.map((v) => v.id))}`).toEqual([]);
      });
    });
  }
}
