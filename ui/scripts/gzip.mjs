// Writes a .gz sibling next to every compressible build asset. The Go server
// serves it when the client accepts gzip. index.html is skipped: the server
// rewrites it on every response.
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { gzipSync, constants } from 'node:zlib';

const dist = new URL('../dist/', import.meta.url).pathname;
let count = 0;

async function walk(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      await walk(path);
    } else if (/\.(js|css|svg|json|txt)$/.test(entry.name)) {
      const raw = await readFile(path);
      if (raw.length < 1024) continue;
      await writeFile(`${path}.gz`, gzipSync(raw, { level: constants.Z_BEST_COMPRESSION }));
      count += 1;
    }
  }
}

await walk(dist);
console.log(`gzip: wrote ${count} files`);
