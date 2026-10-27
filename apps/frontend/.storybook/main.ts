import type { StorybookConfig } from '@storybook/sveltekit';
import { fileURLToPath } from 'node:url';

const config: StorybookConfig = {
  // Permit the workspace proxy hostname while keeping the listener on loopback.
  core: {
    allowedHosts: process.env.PASEO_URL ? [new URL(process.env.PASEO_URL).hostname] : undefined
  },
  stories: ['../src/**/*.mdx', '../src/**/*.stories.@(js|ts|svelte)'],
  addons: [
    '@storybook/addon-svelte-csf',
    '@storybook/addon-docs',
    '@storybook/addon-a11y',
    '@storybook/addon-vitest'
  ],
  framework: {
    name: '@storybook/sveltekit',
    options: {}
  },
  viteFinal(config) {
    // Storybook loads translations without the app's Vite server configuration.
    config.server = {
      ...config.server,
      fs: {
        ...config.server?.fs,
        allow: [
          ...(config.server?.fs?.allow ?? []),
          fileURLToPath(new URL('../messages', import.meta.url))
        ]
      }
    };
    if (process.env.PASEO_URL) {
      const proxy = new URL(process.env.PASEO_URL);
      // Browser HMR uses the public proxy port, not the loopback listener port.
      config.server = {
        ...config.server,
        hmr: {
          ...(typeof config.server?.hmr === 'object' ? config.server.hmr : {}),
          clientPort: Number(proxy.port || (proxy.protocol === 'https:' ? 443 : 80))
        }
      };
    }
    // Lazy media and diagram imports must be optimized before browser tests start. A
    // dependency reload during a test can leave the Storybook runner stalled.
    config.optimizeDeps = {
      ...config.optimizeDeps,
      include: [
        ...(config.optimizeDeps?.include ?? []),
        'pele',
        'hls.js',
        'vidstack/player',
        'vidstack/player/layouts',
        'vidstack/player/ui'
      ]
    };
    return config;
  }
};
export default config;
