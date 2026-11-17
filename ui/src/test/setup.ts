process.env.TZ = 'UTC';

import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';

import { clearAppQueryCache } from '../app/App';

// jsdom does not implement layout, so scrollTo is a no-op; Shell calls it on
// every route change.
window.scrollTo = () => undefined;

// App.tsx keeps one QueryClient for the whole browser session; tests mount
// <App /> repeatedly, so its cache must not leak between cases.
afterEach(() => {
  clearAppQueryCache();
});

