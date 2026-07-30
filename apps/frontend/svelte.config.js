import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { execSync } from 'node:child_process';

const precompress = process.env.CHATTO_FRONTEND_PRECOMPRESS === '1';

function buildVersionName() {
  if (process.env.CHATTO_BUILD_VERSION) return process.env.CHATTO_BUILD_VERSION;
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
      precompress
    }),
    version: {
      // Production image builds inject the same version as the server binary.
      // Other package-script builds use the package version; direct local
      // tooling falls back to the current commit hash.
      name: buildVersionName(),
      // Check for new version every 60 seconds
      pollInterval: 60000
    }
  },
  compilerOptions: {
    fragments: 'tree',
    experimental: {
      async: true
    }
  }
};

export default config;
