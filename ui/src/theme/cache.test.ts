import { createStyleCache, readNonce } from './cache';

describe('style cache', () => {
  it('reads the nonce from the csp-nonce meta tag', () => {
    document.head.innerHTML = '<meta property="csp-nonce" nonce="abc123">';
    expect(readNonce()).toBe('abc123');
  });

  it('returns undefined without the meta tag (vite dev server)', () => {
    document.head.innerHTML = '';
    expect(readNonce()).toBeUndefined();
  });

  it('puts the nonce on the cache and wraps rules in the mui layer', () => {
    const cache = createStyleCache('abc123');
    expect(cache.key).toBe('mui');
    expect(cache.nonce).toBe('abc123');
    const serialized = { name: 'x', styles: 'color:red;' };
    cache.insert('.a', serialized, cache.sheet, false);
    expect(serialized.styles).toBe('@layer mui {color:red;}');
  });

  it('does not nest a layer statement', () => {
    const cache = createStyleCache(undefined);
    const serialized = { name: 'y', styles: '@layer theme, base, mui, components, utilities;' };
    cache.insert('', serialized, cache.sheet, false);
    expect(serialized.styles.startsWith('@layer mui {')).toBe(false);
  });
});
