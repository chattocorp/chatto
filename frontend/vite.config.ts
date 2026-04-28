/// <reference types="vitest/config" />
import devtoolsJson from 'vite-plugin-devtools-json';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import { playwright } from '@vitest/browser-playwright';

// Backend target for dev proxy. Set CHATTO_BACKEND_URL to proxy to a remote
// backend (e.g. "https://dev.chatto.run") instead of a local one.
const backendTarget =
  process.env.CHATTO_BACKEND_URL ||
  `http://localhost:${process.env.CHATTO_WEBSERVER_PORT || '4000'}`;

export default defineConfig({
  clearScreen: false,
  plugins: [tailwindcss(), sveltekit(), devtoolsJson()],
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
