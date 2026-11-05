/** One-shot OAuth callback receiver on an ephemeral literal loopback port. */
import { createServer } from 'node:http';
import { createOAuthFlow, exchangeOAuthCode, isOAuthCallback } from '@chatto/client/auth/oauth';
import { getPublicServerInfo } from '@chatto/client/api/server';
import { nativeServerOrigin } from '@chatto/client/auth/nativeOrigin';
import { registerDesktopRelay } from './url-relay.ts';
import { openMedia } from './media.ts';

/** Run server-owned authorization in the system browser. No password enters this process. */
export async function browserSignIn(
  origin: string,
  clientId: string,
  signal: AbortSignal,
  open = openMedia
) {
  origin = nativeServerOrigin(origin).origin;
  const info = await getPublicServerInfo(origin, { signal });
  if (!info.authorizeUrl) throw new Error('This server does not offer browser sign-in');
  const server = createServer();
  server.requestTimeout = 5000;
  server.headersTimeout = 5000;
  server.maxHeadersCount = 30;
  server.keepAliveTimeout = 1000;
  const deadline = AbortSignal.any([signal, AbortSignal.timeout(5 * 60_000)]);
  let relay: Awaited<ReturnType<typeof registerDesktopRelay>> | undefined;
  try {
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', () => {
        server.removeListener('error', reject);
        resolve();
      });
    });
    const address = server.address();
    if (!address || typeof address === 'string') throw new Error('Could not open sign-in callback');
    const endpoint = `http://127.0.0.1:${address.port}/servers/callback?mode=popup`;
    if (clientId === 'chatto://desktop') relay = await registerDesktopRelay(endpoint);
    const redirectUri = relay?.redirectUri ?? endpoint;
    const flow = await createOAuthFlow({
      origin,
      clientId,
      authorizeUrl: info.authorizeUrl,
      redirectUri
    });
    const callback = new Promise<string>((resolve, reject) => {
      const abort = () => reject(new Error('Browser sign-in cancelled or timed out'));
      deadline.addEventListener('abort', abort, { once: true });
      server.once('close', () => deadline.removeEventListener('abort', abort));
      if (deadline.aborted) {
        abort();
        return;
      }
      let accepted = false;
      server.on('request', async (request, response) => {
        response.setHeader('Cache-Control', 'no-store');
        response.setHeader('Referrer-Policy', 'no-referrer');
        response.setHeader('Content-Security-Policy', "default-src 'none'; frame-ancestors 'none'");
        response.setHeader('Content-Type', 'text/plain; charset=utf-8');
        const raw = request.url ?? '';
        let url = `http://127.0.0.1:${address.port}${raw}`;
        if (relay && request.method === 'POST' && url === endpoint) {
          const chunks: Buffer[] = [];
          let size = 0;
          try {
            for await (const chunk of request) {
              size += chunk.length;
              if (size > 8192) throw new Error();
              chunks.push(Buffer.from(chunk));
            }
            url = Buffer.concat(chunks).toString('utf8');
          } catch {
            response.writeHead(400);
            response.end('Invalid sign-in callback');
            return;
          }
        }
        if (
          accepted ||
          request.method !== (relay ? 'POST' : 'GET') ||
          request.headers.host !== `127.0.0.1:${address.port}` ||
          raw.length > 8192 ||
          !isOAuthCallback(flow, url)
        ) {
          response.writeHead(400);
          response.end('Invalid sign-in callback');
          return;
        }
        accepted = true;
        response.writeHead(200);
        response.end('Sign-in received. You can return to Chatto GPUix.');
        resolve(url);
      });
    });
    // Observe cancellation even if opening the browser rejects first.
    void callback.catch(() => {});
    deadline.throwIfAborted();
    await open(new URL(flow.authorizationUrl));
    const credentials = await exchangeOAuthCode(flow, await callback, { signal: deadline });
    return { credentials, info };
  } finally {
    try {
      await relay?.close();
    } finally {
      server.closeAllConnections();
      await new Promise<void>((resolve) => server.close(() => resolve()));
    }
  }
}
