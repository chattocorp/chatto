/**
 * Svelte 5 adapter for `@chatto/client`.
 *
 * Importing this module makes every client store reactive in Svelte
 * components and runes. Components read store properties directly, for
 * example `store.projection.rooms.get(roomId)`.
 */

import { installSvelteReactivity } from './reactivity.js';

installSvelteReactivity();

export { installSvelteReactivity };
