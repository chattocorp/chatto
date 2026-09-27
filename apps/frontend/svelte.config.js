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

/**
 * Returns the version name that SvelteKit writes to `_app/version.json`.
 *
 * Release builds set `CHATTO_BUILD_VERSION`. The mise `build-frontend` task
 * sets `CHATTO_DEV_BUILD_ID` once per build; together with the workspace name
 * it gives each local build a new version, so open tabs detect the update and
 * do a full reload after a failed navigation. The ID must come from the
 * environment: several processes load this file during one build, and they
 * must all compute the same version. Turbo does not hash either variable, so
 * an unchanged build is restored from its cache with its original version.
 */
function buildVersionName() {
  if (process.env.CHATTO_BUILD_VERSION) return process.env.CHATTO_BUILD_VERSION;

  const base = process.env.npm_package_version ?? gitShortHash();
  const workspace = process.env.CHATTO_DEV_WORKSPACE;
  const buildId = process.env.CHATTO_DEV_BUILD_ID;
  if (workspace && buildId) {
    // The ID format depends on how the task renders it; a short hash keeps the
    // version name compact and free of characters such as spaces or colons.
    const token = createHash('sha256').update(buildId).digest('hex').slice(0, 8);
    return `${base}+${workspace}.${token}`;
  }
  return base;
}

function gitShortHash() {
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
    serviceWorker: {
      register: false
    },
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
        // Opt-in sandboxed HTML attachments can come from any registered server.
        'frame-src': ['self', 'http:', 'https:'],
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
