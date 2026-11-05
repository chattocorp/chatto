/** Run the native design gallery without a server or a saved session. */
import { render_hot } from 'gpuix-svelte';
import { windowDimensions } from './src/window.ts';
render_hot(new URL('./src/Gallery.svelte', import.meta.url), {
  title: 'Chatto — Component gallery',
  ...windowDimensions
});
