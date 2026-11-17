import { render } from '@testing-library/react';

import { App } from '../app/App';
import { stubFetch } from './fetch';

// Renders the whole app at a given route with fetch stubbed from a route
// table. Used by app-shell and page tests so each exercises real routing,
// the real query client and the real theme, not a shallow stand-in.
export function renderApp(route: string, routes: Record<string, [number, unknown]>) {
  const calls = stubFetch(routes);
  window.history.pushState({}, '', route);
  const result = render(<App />);
  return { ...result, calls };
}
