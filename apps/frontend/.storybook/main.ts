import type { StorybookConfig } from '@storybook/sveltekit';

const config: StorybookConfig = {
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
    // Lazy video imports must be optimized before browser tests start. A
    // dependency reload during a test can leave the Storybook runner stalled.
    config.optimizeDeps = {
      ...config.optimizeDeps,
      include: [
        ...(config.optimizeDeps?.include ?? []),
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
