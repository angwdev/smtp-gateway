import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter } from 'react-router';

import { ApiError } from '../api/client';
import { ThemeProviders } from '../theme/Providers';
import { AppRoutes } from './routes';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      refetchOnWindowFocus: true,
      // Retry only what can get better on its own: network errors and 5xx, twice.
      retry: (failures, error) =>
        failures < 2 && error instanceof ApiError && (error.status === 0 || error.status >= 500),
    },
    mutations: { retry: false },
  },
});

// Tests mount <App /> many times against the same module-level client; this
// lets the shared test setup clear cached data between cases.
export function clearAppQueryCache(): void {
  queryClient.clear();
}

export function App() {
  return (
    <ThemeProviders>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <AppRoutes />
        </BrowserRouter>
      </QueryClientProvider>
    </ThemeProviders>
  );
}
