import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { execSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';

const precompress = process.env.CHATTO_FRONTEND_PRECOMPRESS === '1';

/** Returns CSP hashes for hand-written inline scripts in the app template. */
function appTemplateScriptHashes() {
  const template = readFileSync(new URL('./src/app.html', import.meta.url), 'utf8');
  return [...template.matchAll(/<script(?![^>]*\bsrc=)[^>]*>([\s\S]*?)<\/script>/g)].map(
    ([, script]) => `sha256-${createHash('sha256').update(script).digest('base64')}`
  );
}

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
    csp: {
      // The static SPA cannot receive a per-request nonce. SvelteKit adds hashes
      // for the app template and bootstrap scripts to the generated CSP meta tag.
      mode: 'hash',
      directives: {
        'default-src': ['self'],
        'base-uri': ['self'],
        'object-src': ['none'],
        'form-action': ['self'],
        'script-src': ['self', ...appTemplateScriptHashes()],
        // Svelte transitions and several interactive controls create runtime
        // styles or update style attributes.
        'style-src': ['self', 'unsafe-inline'],
        'img-src': ['self', 'data:', 'blob:', 'http:', 'https:'],
        'media-src': ['self', 'blob:', 'http:', 'https:'],
        // A frontend can connect to arbitrary operator-selected Chatto and
        // LiveKit servers, including HTTP development instances.
        'connect-src': ['self', 'http:', 'https:', 'ws:', 'wss:'],
        'frame-src': ['https://www.youtube-nocookie.com'],
        'worker-src': ['self']
      }
    },
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
