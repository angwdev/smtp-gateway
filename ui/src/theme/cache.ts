import createCache, { type EmotionCache } from '@emotion/cache';

// The server writes a fresh nonce into <meta property="csp-nonce"> on every
// response. Browsers hide the attribute from getAttribute, so read the property.
export function readNonce(): string | undefined {
  const meta = document.querySelector<HTMLMetaElement>('meta[property="csp-nonce"]');
  return meta?.nonce || meta?.getAttribute('nonce') || undefined;
}

const LAYER_STATEMENT = /^@layer\s[^{]*$/;

// MUI's StyledEngineProvider can put styles in a cascade layer but cannot set
// a CSP nonce, so the cache is built here. It does what the provider does with
// enableCssLayer: every rule is wrapped in @layer mui.
export function createStyleCache(nonce: string | undefined): EmotionCache {
  const cache = createCache({ key: 'mui', nonce });
  const insert = cache.insert;
  cache.insert = (selector, serialized, sheet, shouldCache) => {
    if (!LAYER_STATEMENT.test(serialized.styles)) {
      serialized.styles = `@layer mui {${serialized.styles}}`;
    }
    return insert(selector, serialized, sheet, shouldCache);
  };
  return cache;
}
