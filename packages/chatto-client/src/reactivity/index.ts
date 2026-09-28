/**
 * Framework-neutral reactivity used by every Chatto client store.
 * UI adapters make reads outside computeds and effects observable; see
 * `@chatto/client/svelte`.
 */

export {
  batch,
  computed,
  effect,
  effectRoot,
  isObservingReads,
  setReadHook,
  signal,
  subscribe,
  untrack,
  type Computed,
  type Equality,
  type ReactiveOptions,
  type ReactiveSource,
  type ReadHook,
  type Signal
} from './core.js';
export { ReactiveMap, ReactiveSet } from './collections.js';
