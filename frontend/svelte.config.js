import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { execSync } from 'node:child_process';

function buildVersionName() {
  if (process.env.npm_package_version) return process.env.npm_package_version;

  try {
    return execSync('git rev-parse --short HEAD', { encoding: 'utf8' }).trim();
  } catch {
    return 'dev';
  }
}

/** @type {import('@sveltejs/kit').Config} */
const config = {
  // Consult https://svelte.dev/docs/kit/integrations
  // for more information about preprocessors
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      fallback: '200.html',
      precompress: true
    }),
    version: {
      // Use package version when run through package scripts, or the current
      // commit hash when launched directly by local dev tooling.
      name: buildVersionName(),
      // Check for new version every 60 seconds
      pollInterval: 60000
    }
  },
  compilerOptions: {
    experimental: {
      async: true
    }
  }
};

export default config;
