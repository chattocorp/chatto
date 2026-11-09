/** Development entry: the native window reloads when a Svelte component changes. */
import { render_hot } from 'gpuix-svelte';
import { windowDimensions } from './src/window.ts';

render_hot(new URL('./src/App.svelte', import.meta.url), {
  title: 'Chatto GPUix — Experiment 2',
  ...windowDimensions
});
