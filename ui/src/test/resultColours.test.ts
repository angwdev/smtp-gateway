import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Guards the result-colour rule in DESIGN.md: "The three result colours
// appear only on a result (a status mark, a chart bar, a count). Nothing
// else is green or amber." MUI's success/warning map to sent/rejected
// (theme.ts), so an Alert with severity="success" or "warning" anywhere
// outside a page that is rendering an actual message result would put
// green or amber on a non-result notice. Non-result notices use <Notice>
// (Notice.tsx) instead, which never carries a result colour.
const ALLOWED_FILES = new Set([
  // Shows the colour of the message's own delivery result (failed/rejected)
  // on its own detail page: a legitimate result, not a generic notice.
  'MessageDetail.tsx',
]);

const SRC_DIR = join(__dirname, '..');

function* walk(dir: string): Generator<string> {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === 'test') continue;
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      yield* walk(full);
    } else if (entry.isFile() && /\.tsx?$/.test(entry.name) && !entry.name.endsWith('.test.tsx') && !entry.name.endsWith('.test.ts')) {
      yield full;
    }
  }
}

describe('result-colour discipline', () => {
  it('never uses severity="success" or severity="warning" outside an allowed file', () => {
    const offenders: string[] = [];
    for (const file of walk(SRC_DIR)) {
      if (ALLOWED_FILES.has(file.split('/').pop() as string)) continue;
      const text = readFileSync(file, 'utf8');
      if (/severity=["']success["']|severity=["']warning["']/.test(text)) offenders.push(file);
    }
    expect(offenders, `found a result-colour Alert outside an allowed file: ${offenders.join(', ')}`).toEqual([]);
  });
});
