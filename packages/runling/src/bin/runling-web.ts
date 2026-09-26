#!/usr/bin/env node

import { serverLog, serverLogPath } from '../runtime/server-log.ts';
import { installShutdown } from '../runtime/shutdown.ts';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { createServer } from 'vite';
import { CommanderError } from 'commander';
import { createServeCommand, type ServeOptions } from '../runtime/cli.ts';

export async function runRunlingWeb(options: ServeOptions): Promise<void> {
  if (existsSync('.env')) process.loadEnvFile('.env');
  const appRoot = resolve(import.meta.dirname, '../..');
  const configPath = resolve(options.config);
  process.env.RUNLING_WEB_CONFIG = configPath;
  process.env.RUNLING_WATCH = options.watch ? '1' : '0';

  serverLog('info', 'server.starting', {
    config: configPath,
    logFile: serverLogPath(),
    port: options.port
  });

  // SvelteKit resolves its app files from the process directory, even with Vite's
  // root set. This is the tooling root; tasks still require explicit directories.
  process.chdir(appRoot);

  const server = await createServer({
    root: appRoot,
    configFile: resolve(appRoot, 'vite.config.ts'),
    clearScreen: false,
    ssr: { resolve: { externalConditions: ['runling-source'] } },
    server: {
      host: options.host,
      open: options.open,
      port: options.port,
      strictPort: true
    }
  });

  const sourceHost = (await server.ssrLoadModule('/src/lib/server/source-host.ts')) as {
    startSourceHost(): Promise<unknown>;
    stopSourceHost(): Promise<void>;
  };
  try {
    await sourceHost.startSourceHost();
    await server.listen();
  } catch (error) {
    await sourceHost.stopSourceHost();
    await server.close();
    throw error;
  }
  server.printUrls();
  serverLog('info', 'server.listening', { host: options.host, port: options.port });
  server.httpServer?.once('close', () => serverLog('info', 'server.stopped'));
  installShutdown(async () => {
    await sourceHost.stopSourceHost();
    await server.close();
  });
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  try {
    await createServeCommand()
      .name('runling-web')
      .exitOverride()
      .action(runRunlingWeb)
      .parseAsync(process.argv);
  } catch (cause) {
    if (cause instanceof CommanderError) {
      process.exitCode = cause.exitCode;
    } else {
      serverLog('error', 'server.start_failed', { error: cause });
      process.exitCode = 1;
    }
  }
}
