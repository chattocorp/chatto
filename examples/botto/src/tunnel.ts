/** Own one cloudflared child for the webhook listener's lifetime. */
import { spawn } from 'node:child_process';
import { setTimeout as delay } from 'node:timers/promises';
import { log } from 'runling';

/** The token authorizes a remotely configured tunnel; it never enters process arguments. */
export interface TunnelSettings {
  command: string;
  token: string;
}

/** Restart failed connectors with bounded backoff and reap the child before shutdown completes. */
export async function runTunnel(config: TunnelSettings, signal: AbortSignal): Promise<void> {
  let backoff = 1000;
  while (!signal.aborted) {
    const started = Date.now();
    await new Promise<void>((resolve) => {
      const child = spawn(
        config.command,
        ['tunnel', '--no-autoupdate', '--loglevel', 'error', 'run'],
        {
          // Do not give the connector Chatto, GitHub App, or model-provider credentials.
          env: { PATH: process.env.PATH, HOME: process.env.HOME, TUNNEL_TOKEN: config.token },
          // Connector diagnostics can contain request URLs and credentials. Log lifecycle only.
          stdio: 'ignore'
        }
      );
      let killTimer: ReturnType<typeof setTimeout> | undefined;
      const stop = () => {
        child.kill('SIGTERM');
        killTimer ??= setTimeout(() => child.kill('SIGKILL'), 5000);
      };
      child.once('spawn', () => log.info('Webhook tunnel connector started'));
      child.once('error', () => {}); // The close event also follows a failed spawn.
      child.once('close', () => {
        if (killTimer) clearTimeout(killTimer);
        signal.removeEventListener('abort', stop);
        resolve();
      });
      signal.addEventListener('abort', stop, { once: true });
      if (signal.aborted) stop();
    });
    if (signal.aborted) break;
    if (Date.now() - started >= 60_000) backoff = 1000;
    log.warn('Webhook tunnel connector stopped; retrying', { delayMs: backoff });
    await delay(backoff, undefined, { signal }).catch(() => {});
    backoff = Math.min(backoff * 2, 30_000);
  }
}
