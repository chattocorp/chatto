import { serverLog, serverLogPath } from './server-log.ts';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import type { ServeOptions } from './cli.ts';
import { createServer, type RequestListener } from 'node:http';
import { installShutdown } from './shutdown.ts';

export async function runRunlingWeb(options: ServeOptions) {
  const configPath = resolve(options.config);
  process.env.RUNLING_WEB_CONFIG = configPath;
  process.env.RUNLING_WATCH = options.watch ? '1' : '0';
  serverLog('info', 'server.starting', {
    config: configPath,
    logFile: serverLogPath(),
    port: options.port
  });
  try {
    process.env.HOST = options.host;
    process.env.PORT = String(options.port);
    // Import only after setting the adapter's startup environment. Keep the project cwd.
    const serverUrl = new URL('../../web/handler.js', import.meta.url);
    if (!existsSync(serverUrl))
      throw new Error('Runling web assets are missing. Build the package before running it.');
    const { handler } = (await import(/* @vite-ignore */ serverUrl.href)) as {
      handler: RequestListener;
    };
    const server = createServer(handler);
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject);
      server.listen(options.port, options.host, () => {
        server.off('error', reject);
        resolve();
      });
    });
    const host = options.host.includes(':') ? `[${options.host}]` : options.host;
    const url = `http://${host}:${options.port}`;
    serverLog('info', 'server.listening', { url });
    installShutdown(async () => {
      serverLog('info', 'server.stopping');
      const closed = new Promise<void>((resolve) => server.close(() => resolve()));
      const deadline = setTimeout(() => server.closeAllConnections(), 5000);
      try {
        await (
          globalThis as typeof globalThis & { __runlingStopSources?: () => Promise<void> }
        ).__runlingStopSources?.();
        await closed;
        serverLog('info', 'server.stopped');
      } finally {
        clearTimeout(deadline);
      }
    });
    if (options.open) {
      const { spawn } = await import('node:child_process');
      const child =
        process.platform === 'darwin'
          ? spawn('open', [url])
          : process.platform === 'win32'
            ? spawn('explorer.exe', [url])
            : spawn('xdg-open', [url]);
      child.on('error', () => serverLog('warn', 'server.browser_failed', { url }));
      child.unref();
    }
  } catch (error) {
    await (
      globalThis as typeof globalThis & { __runlingStopSources?: () => Promise<void> }
    ).__runlingStopSources?.();
    serverLog('error', 'server.start_failed', { error });
    throw error;
  }
}
