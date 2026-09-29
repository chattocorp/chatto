import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from 'vitest';

test('loads no stateful client code at runtime', () => {
  // The root of @chatto/client loads the registry and the stores. A stateless
  // host, such as a webhook handler with createBotApi, must not pay for them.
  const directory = join(import.meta.dirname);
  const sources = readdirSync(directory).filter(
    (file) => file.endsWith('.ts') && !file.endsWith('.test.ts')
  );
  for (const file of sources) {
    const source = readFileSync(join(directory, file), 'utf8');
    expect(source, file).not.toMatch(/^import (?!type )[^;]*from '@chatto\/client';/m);
  }
});
