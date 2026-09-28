/**
 * Bridge from Chatto client reactivity to Svelte 5 runes.
 *
 * Svelte tracks a client value when a component, `$derived`, or `$effect`
 * reads it. Each read outside a client computed or effect reaches the read
 * hook. The hook uses one `createSubscriber` per reactive node: Svelte
 * subscribes only while a tracking reaction reads the node, and stops the
 * client subscription when the last reaction is destroyed.
 */

import { createSubscriber } from 'svelte/reactivity';
import { setReadHook, subscribe, type ReactiveSource } from '../reactivity/index.js';

const subscribers = new WeakMap<ReactiveSource, () => void>();

function trackInSvelte(source: ReactiveSource): void {
  let subscribeInSvelte = subscribers.get(source);
  if (!subscribeInSvelte) {
    subscribeInSvelte = createSubscriber((update) => subscribe(source, update));
    subscribers.set(source, subscribeInSvelte);
  }
  // Outside a tracking Svelte reaction this is a cheap no-op.
  subscribeInSvelte();
}

let installed = false;

/**
 * Make client state reactive in Svelte. Importing `@chatto/client/svelte`
 * installs the bridge; calling this again has no effect.
 */
export function installSvelteReactivity(): void {
  if (installed) return;
  installed = true;
  setReadHook(trackInSvelte);
}
