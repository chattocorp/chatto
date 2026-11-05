/** Bundle the native app for the current platform. This entry requires Bun. */
import { build } from 'gpuix-svelte/build';

await build({ entry: './standalone.ts' });
