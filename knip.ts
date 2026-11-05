import type { KnipConfig } from 'knip';

export default {
  // GPUix bundles its Svelte fork instead of declaring the standard package.
  // Trace script imports even when Knip cannot auto-enable its Svelte compiler.
  compilers: {
    svelte: (source: string) =>
      [...source.matchAll(/<script\b(?:[^>"']|"[^"]*"|'[^']*')*>([\s\S]*?)<\/script>/gi)]
        .map((match) => match[1] ?? '')
        .join('\n')
  },
  rules: {
    dependencies: 'error',
    devDependencies: 'error',
    files: 'error',
    unlisted: 'error',
    exports: 'off',
    types: 'off',
    enumMembers: 'off',
    namespaceMembers: 'off',
    duplicates: 'error'
  },
  ignoreBinaries: ['mise', 'playwright'],
  ignoreDependencies: ['gemoji'],
  workspaces: {
    '.': {
      // The lifecycle tests launch the fixture in a child process.
      entry: ['tools/*.mjs', 'tools/fixtures/paseo-lifecycle.mjs'],
      project: ['tools/**/*.mjs'],
      // These commands come from mise, the operating system, or Paseo.
      ignoreBinaries: ['go', 'buf', 'gofmt', 'traefik', 'paseo', 'python3'],
      ignoreDependencies: [
        // createRequire in check-server-compatibility.mjs resolves semver from the client package.
        'semver',
        // Root Prettier resolves the frontend configuration's plugins from the root working directory.
        'prettier-plugin-tailwindcss'
      ]
    },
    'apps/frontend': {
      entry: [
        'playwright.compat.config.ts',
        'e2e/remote-compatibility.compat.ts',
        'scripts/*.mjs',
        // Svelte template imports and ambient Window declarations are not traced by Knip.
        'src/lib/components/messages/EntityLinkMenu.svelte',
        'src/lib/desktop/bridge.ts'
      ],
      project: [
        '.storybook/**/*.{css,js,ts}',
        'e2e/**/*.ts',
        'scripts/**/*.mjs',
        'src/**/*.{js,ts,svelte}'
      ],
      ignoreDependencies: ['@iconify/json', 'svelte2tsx'],
      ignoreIssues: {
        'src/lib/i18n/messages.ts': ['exports', 'types'],
        'src/lib/pb/**': ['exports', 'types', 'enumMembers'],
        // The default settings destination intentionally aliases the profile destination.
        'e2e/routes.ts': ['duplicates']
      }
    },
    'apps/docs-website': {
      entry: ['src/custom.css', 'src/routeData.ts', 'src/generated/connectrpc-api/*.raw.mdx']
    },
    'apps/desktop': {
      // bundle-updater.mjs passes updater_vendor.mjs to esbuild as an entry point.
      entry: ['preload.cjs', 'updater_vendor.mjs', 'scripts/*.mjs'],
      project: ['**/*.{cjs,mjs}'],
      ignoreBinaries: ['xcrun', 'pwsh']
    },
    'experimental/chatto-gpuix': {
      // GPUix loads root components by URL, outside the standard SvelteKit routes.
      entry: [
        'app.ts',
        'gallery.ts',
        'standalone.ts',
        'src/App.svelte',
        'src/Gallery.svelte',
        'tests/{app,browser-live,gallery,live,media,scale,session,threads,threads-live}.ts'
      ],
      project: ['*.{ts,js}', 'src/**/*.{ts,svelte}', 'tests/**/*.{ts,svelte}'],
      svelte: true,
      sveltekit: false
    },
    'packages/api-types': {
      ignoreDependencies: ['@bufbuild/protoc-gen-es', '@connectrpc/protoc-gen-connect-es']
    },
    'packages/translatto': {
      entry: ['src/**/*.spec.ts', 'test/**/*.ts'],
      project: ['src/**/*.ts', 'test/**/*.ts']
    },
    'packages/chattobot': {
      // Evaluation scripts pass task paths to the Runling CLI.
      entry: ['runling.config.ts', 'tasks/evaluate-*.ts', '**/*.test.ts'],
      project: ['**/*.ts'],
      ignoreBinaries: ['gh']
    },
    'examples/botto': {
      entry: ['runling.config.ts']
    },
    'examples/runling-bot': {
      entry: ['runling.config.ts']
    },
    'packages/runling': {
      // The CLI loads configs and examples by path; subprocess tests load fixtures and scripts.
      entry: ['runling.config.ts', 'examples/*.ts', 'test/fixtures/*.ts', 'scripts/*.mjs'],
      // Iconify loads this collection from icon-[lucide--...] CSS classes.
      ignoreDependencies: ['@iconify-json/lucide'],
      ignoreIssues: {
        // Tasks have a named export for composition and a default export for CLI execution.
        'examples/{ask,implement,joke,plan,research,review}.ts': ['duplicates']
      }
    }
  }
} satisfies KnipConfig;
