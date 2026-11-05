/** Development entry: the native window reloads when a Svelte component changes. */
import { render_hot } from 'gpuix-svelte';

render_hot(new URL('./src/App.svelte', import.meta.url), {
  title: 'Chatto GPUix — Experiment 2',
  width: 1100,
  height: 740
});
