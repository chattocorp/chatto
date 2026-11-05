/** Run the native design gallery without a server or a saved session. */
import { render_hot } from 'gpuix-svelte';
render_hot(new URL('./src/Gallery.svelte', import.meta.url), {
  title: 'Chatto — Component gallery',
  width: 1100,
  height: 740
});
