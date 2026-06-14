/// <reference types="vitest/config" />
import { readFileSync } from 'node:fs';
import devtoolsJson from 'vite-plugin-devtools-json';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, type Plugin } from 'vite';
import { playwright } from '@vitest/browser-playwright';
import { parse } from 'graphql';

// Backend target for dev proxy. Set CHATTO_BACKEND_URL to proxy to a remote
// backend (e.g. "https://dev.chatto.run") instead of a local one.
const backendTarget =
  process.env.CHATTO_BACKEND_URL ||
  `http://localhost:${process.env.CHATTO_WEBSERVER_PORT || '4000'}`;
const enableGraphqlCodegenClientOptimizer =
  process.env.CHATTO_DISABLE_GRAPHQL_CODEGEN_OPTIMIZER !== '1';

function graphqlCodegenClientOptimizer(): Plugin {
  const graphqlCallPattern = /\bgraphql\s*\(\s*`([\s\S]*?)`\s*\)/g;
  const generatedModule = '$lib/gql/graphql';
  let generatedDocuments: Map<string, string> | null = null;

  function escapeRegExp(value: string): string {
    return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  }

  function loadGeneratedDocuments(): Map<string, string> {
    if (generatedDocuments) return generatedDocuments;

    const gqlSource = readFileSync(new URL('./src/lib/gql/gql.ts', import.meta.url), 'utf8');
    const generatedDocumentPattern = /"((?:\\.|[^"\\])*)": (?:typeof )?types\.(\w+)/g;
    const documents = new Map<string, string>();
    let match: RegExpExecArray | null;

    while ((match = generatedDocumentPattern.exec(gqlSource))) {
      const [, documentSource, exportedName] = match;
      documents.set(JSON.parse(`"${documentSource}"`), exportedName);
    }

    generatedDocuments = documents;
    return generatedDocuments;
  }

  function documentExportName(documentSource: string, id: string): string {
    const generatedName = loadGeneratedDocuments().get(documentSource);
    if (generatedName) return generatedName;

    const document = parse(documentSource);
    const definition = document.definitions.find(
      (candidate) =>
        candidate.kind === 'OperationDefinition' || candidate.kind === 'FragmentDefinition'
    );

    if (!definition?.name?.value) {
      throw new Error(`Anonymous GraphQL documents cannot be optimized in ${id}`);
    }

    return definition.kind === 'FragmentDefinition'
      ? `${definition.name.value}FragmentDoc`
      : `${definition.name.value}Document`;
  }

  function hasValueImport(code: string, importedName: string): boolean {
    const importPattern = /import\s+(?!type\b)([\s\S]*?)\s+from\s+['"]([^'"]+)['"];?/g;
    let match: RegExpExecArray | null;

    while ((match = importPattern.exec(code))) {
      const [, specifier, source] = match;
      if (!source.endsWith('/gql/graphql') && source !== generatedModule) continue;
      if (new RegExp(`\\b${importedName}\\b`).test(specifier)) return true;
    }

    return false;
  }

  function hasLocalDeclaration(code: string, localName: string): boolean {
    return new RegExp(
      `\\b(?:const|let|var|function|class)\\s+${escapeRegExp(localName)}\\b`
    ).test(code);
  }

  function localImportNameFor(code: string, importedName: string): string {
    if (!hasLocalDeclaration(code, importedName)) return importedName;

    let candidate = `__Graphql${importedName}`;
    let suffix = 2;
    while (new RegExp(`\\b${escapeRegExp(candidate)}\\b`).test(code)) {
      candidate = `__Graphql${importedName}${suffix}`;
      suffix += 1;
    }

    return candidate;
  }

  return {
    name: 'chatto-graphql-codegen-client-optimizer',
    enforce: 'post',
    transform(code, id) {
      const [filename] = id.split('?');

      if (
        !filename ||
        filename.includes('/node_modules/') ||
        filename.includes('/.svelte-kit/') ||
        filename.includes('/src/lib/gql/') ||
        (!filename.endsWith('.ts') && !filename.endsWith('.svelte'))
      ) {
        return null;
      }

      if (!code.includes('graphql(`')) return null;

      const imports = new Map<string, string>();
      const transformed = code.replace(graphqlCallPattern, (_match, documentSource: string) => {
        const importedName = documentExportName(documentSource, id);
        const localName = hasValueImport(code, importedName)
          ? importedName
          : localImportNameFor(code, importedName);
        imports.set(importedName, localName);
        return localName;
      });

      if (transformed === code) return null;

      const missingImports = [...imports]
        .filter(([importedName]) => !hasValueImport(code, importedName))
        .sort();
      const importBlock = missingImports.length
        ? `import { ${missingImports
            .map(([importedName, localName]) =>
              importedName === localName ? importedName : `${importedName} as ${localName}`
            )
            .join(', ')} } from '${generatedModule}';\n`
        : '';

      return {
        code: `${importBlock}${transformed}`,
        map: null
      };
    }
  };
}

export default defineConfig({
  clearScreen: false,
  plugins: [
    tailwindcss(),
    sveltekit(),
    ...(enableGraphqlCodegenClientOptimizer ? [graphqlCodegenClientOptimizer()] : []),
    devtoolsJson()
  ],
  build: {
    rollupOptions: {
      output: {
        experimentalMinChunkSize: 20_000
      }
    }
  },
  ssr: {
    // TipTap is browser-only but imported in Svelte components that are
    // compiled for SSR. Bundle them into the SSR output to avoid
    // "could not be resolved" warnings (the code paths are guarded by
    // $effect which doesn't run during SSR).
    noExternal: ['@tiptap/core', '@tiptap/starter-kit', '@tiptap/extension-placeholder']
  },
  optimizeDeps: {
    exclude: ['@urql/svelte']
  },
  server: {
    // Proxy some URL routes to the Go backend process in development.
    port: process.env.VITE_PORT ? parseInt(process.env.VITE_PORT) : undefined,
    host: true,
    allowedHosts: ['fatso.fritz.box', '.orb.local'],
    // Bind-mount inotify on macOS (Docker Desktop / OrbStack) drops events
    // during bursty changes. Polling is reliable; cost is negligible at this
    // tree size.
    watch: {
      usePolling: true,
      interval: 300
    },
    proxy: {
      '/playground': {
        target: backendTarget,
        changeOrigin: true
      },
      '/api': {
        target: backendTarget,
        ws: true,
        changeOrigin: true,
        secure: false,
        cookieDomainRewrite: { '*': '' },
        // Rewrite the Origin header on WebSocket upgrades so the
        // backend's CheckOrigin accepts the connection.
        rewriteWsOrigin: true
      },
      '/auth': {
        target: backendTarget,
        changeOrigin: true,
        cookieDomainRewrite: { '*': '' }
      },
      '/assets': {
        target: backendTarget,
        changeOrigin: true
      },
      '/webhooks': {
        target: backendTarget,
        changeOrigin: true
      }
    }
  },
  test: {
    expect: { requireAssertions: true },
    projects: [
      {
        extends: './vite.config.ts',
        test: {
          name: 'client',
          browser: {
            enabled: true,
            provider: playwright(),
            headless: !process.env.SHOW_BROWSER,
            instances: [{ browser: 'chromium' }]
          },
          include: ['src/**/*.svelte.{test,spec}.{js,ts}'],
          exclude: ['src/lib/server/**'],
          setupFiles: ['./vitest-setup-client.ts'],
          deps: {
            // Pre-bundle Shiki theme packages for dynamic import in browser tests
            optimizer: {
              web: {
                include: ['@shikijs/themes/github-light', '@shikijs/themes/nord']
              }
            }
          }
        }
      },
      {
        extends: './vite.config.ts',
        test: {
          name: 'server',
          environment: 'node',
          include: ['src/**/*.{test,spec}.{js,ts}'],
          exclude: ['src/**/*.svelte.{test,spec}.{js,ts}'],
          testTimeout: 10000 // CI is slower with Svelte module transforms
        }
      }
    ]
  }
});
