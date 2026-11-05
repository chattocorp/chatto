/** Static entry used both by the production runner and the standalone app build. */
import { render } from 'gpuix-svelte';
import App from './src/App.svelte';

render(App, {
  title: 'Chatto GPUix',
  width: 1100,
  height: 740,
  app: {
    name: 'Chatto GPUix',
    identifier: 'run.chatto.gpuix.experimental',
    copyright: 'ChattoCorp GmbH'
  }
});
