import '@fontsource-variable/atkinson-hyperlegible-next/wght.css';
import '@fontsource-variable/atkinson-hyperlegible-mono/wght.css';
import '../index.css';

import { CacheProvider } from '@emotion/react';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider } from '@mui/material/styles';
import type { ReactNode } from 'react';

import { createStyleCache, readNonce } from './cache';
import { theme } from './theme';

const cache = createStyleCache(readNonce());

export function ThemeProviders({ children }: { children: ReactNode }) {
  return (
    <CacheProvider value={cache}>
      <ThemeProvider theme={theme} defaultMode="system" noSsr>
        <CssBaseline />
        {children}
      </ThemeProvider>
    </CacheProvider>
  );
}
