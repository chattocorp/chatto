/** Native login UI and host cancellation never need a real password or network. */
import { mkdtempSync, mkdirSync, rmSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import {
  check,
  click_test_id,
  finish,
  mount_headless,
  wait_for,
  all_text
} from 'gpuix-svelte/test';
import App from '../src/App.svelte';
import { NativeSession } from '../src/session.svelte.ts';
import { safeMarkdown } from '../src/markdown.ts';

const base = fileURLToPath(new URL('../../../.context/', import.meta.url));
mkdirSync(base, { recursive: true });
const directory = mkdtempSync(join(base, 'gpuix-session-'));
process.env.CHATTO_GPUIX_CONFIG_DIR = directory;
let requests = 0;
const originalFetch = globalThis.fetch;
globalThis.fetch = async () => {
  requests++;
  throw new Error('A native login cancellation must not send a request');
};
let prompts = 0;
const session = new NativeSession({
  password: async () => {
    prompts++;
    return null;
  }
});
session.login = 'synthetic';
const mounted = mount_headless(App, { props: { session }, width: 900, height: 500 });
try {
  check('opens a real sign-in screen', all_text().includes('Sign in to Chatto'));
  click_test_id('sign-in');
  await wait_for(() => !session.busy);
  check('sign-in uses the injected secure-prompt boundary', prompts, 1);
  check('cancelling a prompt does not connect', session.server, null);
  check('cancelling a prompt sends no request', requests, 0);
  let complete: (value: string | null) => void = () => {};
  const pending = new NativeSession({
    password: () => new Promise((resolve) => (complete = resolve))
  });
  pending.login = 'synthetic';
  const signIn = pending.signIn();
  pending.close();
  complete('synthetic-password');
  await signIn;
  check('late password delivery cannot revive a closed host', pending.server, null);
  check('late delivery sends no credential request', requests, 0);
  const invalid = new NativeSession({ password: async () => 'synthetic-password' });
  invalid.origin = 'https://example.test/path';
  invalid.login = 'synthetic';
  await invalid.signIn();
  check('invalid origins fail before transmission', requests, 0);
  check('invalid origin failure is visible', invalid.error.length > 0);
  invalid.close();
  const safe = safeMarkdown(
    '**Hello** ![alt](https://example.test/image.png)\n\n![ref][pic]\n\n[pic]: https://example.test/ref.png\n\n<img src="https://example.test/html.png">'
  );
  check('native Markdown keeps emphasis', safe.includes('**Hello**'));
  check(
    'inline and reference images are text labels',
    safe.includes('Image: alt') && safe.includes('Image: ref')
  );
  check('raw HTML becomes a code block', safe.includes('```') || safe.includes('~~~'));
} finally {
  mounted.unmount();
  session.close();
  globalThis.fetch = originalFetch;
  rmSync(directory, { recursive: true, force: true });
}
finish('Chatto GPUix session', 11);
