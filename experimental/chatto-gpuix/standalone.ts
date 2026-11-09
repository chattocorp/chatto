/** Static entry used both by the production runner and the standalone app build. */
import { render } from 'gpuix-svelte';
import { windowDimensions } from './src/window.ts';
import App from './src/App.svelte';

render(App, {
  title: 'Chatto GPUix',
  // GPUix's packager reads this object statically and does not accept spreads.
  width: windowDimensions.width,
  height: windowDimensions.height,
  minWidth: windowDimensions.minWidth,
  minHeight: windowDimensions.minHeight,
  app: {
    name: 'Chatto GPUix',
    identifier: 'run.chatto.gpuix.experimental',
    copyright: 'ChattoCorp GmbH'
  }
});
