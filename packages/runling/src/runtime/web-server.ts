/** Host the console and workflows in the application's process. */
import { serverLog, serverLogPath } from './server-log.ts';
import { existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createServer, type RequestListener } from 'node:http';
import { installShutdown } from './shutdown.ts';
import { ConfigReloader } from './config-reloader.ts';
import type { WebConfig } from './web-config.ts';
import type { ServeOptions } from './cli.ts';

/** @internal Shared with the packaged console so sources use the application's log context. */
export { SourceManager } from './source-manager.ts';

/** One web host can run in a process. Configuration objects are loaded once. */
export interface ServerOptions {
  config: WebConfig | string;
  /** Base for file configuration and default storage. Sources keep the caller's cwd. */
  directory?: string;
  dataDirectory?: string;
  host?: string;
  port?: number;
  watch?: boolean;
  quietLog?: boolean;
  /** Install SIGINT and SIGTERM cleanup. Default: true. */
  signals?: boolean;
}

/** A listening Runling host. close() is idempotent and waits for source and task cleanup. */
export interface RunlingServer extends AsyncDisposable {
  url: string;
  close(): Promise<void>;
}

const state = globalThis as typeof globalThis & {
  __runlingWebServer?: boolean;
  __runlingConfigReloader?: ConfigReloader;
  __runlingStartSources?: () => Promise<unknown>;
  __runlingStopSources?: () => Promise<void>;
};

/** Start the web console with an object or configuration file. No child process.
 * All sources, task events, journals, and HTTP routes share the installed Runling runtime.
 * Workspace source uses Vite, which temporarily changes cwd during initialization. */
export async function serve(options: ServerOptions): Promise<RunlingServer> {
  if (state.__runlingWebServer) throw new Error('A Runling server already owns this process');
  const port = options.port ?? 5173;
  if (!Number.isInteger(port) || port < 0 || port > 65535)
    throw new RangeError('Invalid server port');
  if (typeof options.config !== 'string' && options.watch)
    throw new Error('Watching requires a configuration file');
  state.__runlingWebServer = true;
  const directory = resolve(options.directory ?? process.cwd());
  const configPath =
    typeof options.config === 'string'
      ? resolve(directory, options.config)
      : join(directory, 'runling.config.ts');
  const host = options.host ?? '127.0.0.1';
  const env = {
    RUNLING_WEB_CONFIG: configPath,
    RUNLING_WATCH: options.watch ? '1' : '0',
    RUNLING_QUIET_LOG: options.quietLog ? '1' : '0',
    ...(options.dataDirectory
      ? { RUNLING_DATA_DIRECTORY: resolve(directory, options.dataDirectory) }
      : {})
  };
  const previous = new Map(Object.keys(env).map((key) => [key, process.env[key]]));
  previous.set('RUNLING_DATA_DIRECTORY', process.env.RUNLING_DATA_DIRECTORY);
  Object.assign(process.env, env);
  let detach: (() => void) | undefined;
  let server: ReturnType<typeof createServer> | undefined;
  let development: { close(): Promise<void> } | undefined;
  let closing: Promise<void> | undefined;
  const close = () =>
    (closing ??= (async () => {
      const closed = server?.listening
        ? new Promise<void>((resolve) => server!.close(() => resolve()))
        : Promise.resolve();
      const deadline = setTimeout(() => server?.closeAllConnections(), 5000);
      try {
        try {
          await state.__runlingStopSources?.();
        } finally {
          try {
            await state.__runlingConfigReloader?.close();
          } finally {
            try {
              await development?.close();
            } finally {
              await closed;
            }
          }
        }
        serverLog('info', 'server.stopped');
      } finally {
        detach?.();
        clearTimeout(deadline);
        delete state.__runlingConfigReloader;
        delete state.__runlingWebServer;
        for (const [key, value] of previous) {
          if (value === undefined) delete process.env[key];
          else process.env[key] = value;
        }
      }
    })());
  try {
    state.__runlingConfigReloader = new ConfigReloader(
      configPath,
      typeof options.config === 'string' ? { watch: options.watch } : { config: options.config }
    );
    // Project setup must finish before logs, persistence, or sources open their resources.
    await state.__runlingConfigReloader.load();
    serverLog('info', 'server.starting', { config: configPath, logFile: serverLogPath(), port });
    let handler: RequestListener;
    if (fileURLToPath(import.meta.url).endsWith('.ts')) {
      // Source consumers use the console directly. Published installs never import Vite.
      const { createServer: createViteServer } = await import('vite');
      const appRoot = fileURLToPath(new URL('../../', import.meta.url));
      const previousDirectory = process.cwd();
      let vite;
      try {
        // SvelteKit resolves its app files from cwd during Vite setup.
        process.chdir(appRoot);
        vite = await createViteServer({
          configFile: join(appRoot, 'vite.config.ts'),
          clearScreen: false,
          server: { middlewareMode: true, hmr: false }
        });
      } finally {
        process.chdir(previousDirectory);
      }
      development = vite;
      const sourceHost = (await vite.ssrLoadModule('/src/lib/server/source-host.ts')) as {
        startSourceHost(): Promise<unknown>;
        stopSourceHost(): Promise<void>;
      };
      state.__runlingStartSources = sourceHost.startSourceHost;
      state.__runlingStopSources = sourceHost.stopSourceHost;
      handler = vite.middlewares;
    } else {
      const serverUrl = new URL('../../web/handler.js', import.meta.url);
      if (!existsSync(serverUrl))
        throw new Error('Runling web assets are missing. Build the package before running it.');
      ({ handler } = (await import(/* @vite-ignore */ serverUrl.href)) as {
        handler: RequestListener;
      });
    }
    // Adapter init runs once per imported module; explicit startup also supports a later host.
    await state.__runlingStartSources?.();
    server = createServer(handler);
    await new Promise<void>((resolve, reject) => {
      server!.once('error', reject);
      server!.listen(port, host, () => {
        server!.off('error', reject);
        resolve();
      });
    });
    const address = server.address();
    if (!address || typeof address === 'string') throw new Error('Server has no TCP address');
    const url = `http://${host.includes(':') ? `[${host}]` : host}:${address.port}`;
    serverLog('info', 'server.listening', { url });
    if (options.signals !== false) detach = installShutdown(close);
    return { url, close, [Symbol.asyncDispose]: close };
  } catch (error) {
    await close();
    throw error;
  }
}

/** CLI adapter for the same programmatic host. */
export async function runRunlingWeb(options: ServeOptions): Promise<void> {
  const server = await serve(options);
  if (options.open) {
    const { spawn } = await import('node:child_process');
    const child = spawn(
      process.platform === 'darwin'
        ? 'open'
        : process.platform === 'win32'
          ? 'explorer.exe'
          : 'xdg-open',
      [server.url]
    );
    child.on('error', () => serverLog('warn', 'server.browser_failed', { url: server.url }));
    child.unref();
  }
}
