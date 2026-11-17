import tailwindcss from '@tailwindcss/vite';
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// The Go server replaces this placeholder with a fresh nonce on every response.
const CSP_NONCE_PLACEHOLDER = '__CSP_NONCE__';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  html: { cspNonce: CSP_NONCE_PLACEHOLDER },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    sourcemap: false,
    // Never inline fonts or images as data: URLs. The CSP only allows font-src 'self'.
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:18080',
      // A plain string match is a prefix match, which would also catch the
      // SPA's own /logins routes; bypass back to Vite for anything that
      // isn't exactly the OIDC sign-in page.
      '/login': {
        target: 'http://127.0.0.1:18080',
        bypass: (req) => {
          if (req.url !== '/login' && !req.url?.startsWith('/login?')) return req.url;
          return undefined;
        },
      },
      '/auth': 'http://127.0.0.1:18080',
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    css: true,
    restoreMocks: true,
    unstubGlobals: true,
  },
});
