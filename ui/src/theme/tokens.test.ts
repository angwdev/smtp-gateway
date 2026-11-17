import css from '../index.css?raw';
import { tokens } from './tokens';

// index.css must declare exactly the values of tokens.ts, so the two can never drift.
function declared(block: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const match of block.matchAll(/--sg-([a-zA-Z0-9]+):\s*(#[0-9a-f]{6});/g)) {
    out[match[1] as string] = match[2] as string;
  }
  return out;
}

describe('colour tokens', () => {
  it('index.css :root matches the light tokens', () => {
    const root = /:root\s*{([^}]*)}/.exec(css)?.[1] ?? '';
    expect(declared(root)).toEqual(tokens.light);
  });

  it('index.css .dark matches the dark tokens', () => {
    const dark = /\.dark\s*{([^}]*)}/.exec(css)?.[1] ?? '';
    expect(declared(dark)).toEqual(tokens.dark);
  });

  it('light and dark define the same names', () => {
    expect(Object.keys(tokens.dark).sort()).toEqual(Object.keys(tokens.light).sort());
  });
});
