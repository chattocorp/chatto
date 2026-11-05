/** Direct native-host password sign-in. Credentials and error bodies are never logged. */
import { directBearerSession, type NewBearerSession } from './bearerSession.js';
import { parseServerUrl } from '../util/serverUrl.js';

/** A private, bounded sign-in failure. The server's response text is never exposed. */
export class PasswordLoginError extends Error {
  constructor(readonly status: number | null = null) {
    super(
      status === 401
        ? 'Incorrect username or password'
        : status === 403
          ? 'Password sign-in is not permitted on this server'
          : 'Could not sign in to this server'
    );
    this.name = 'PasswordLoginError';
  }
}

/**
 * Obtain a renewable human session on servers that permit direct password login.
 * Uses HTTPS, or HTTP on loopback only. The origin must have no path, query, or
 * fragment. No cookies, redirects, automatic retries, or password retention.
 * Native hosts own secure input; register the returned session with the client.
 */
export async function passwordLogin(
  serverUrl: string,
  input: { login: string; password: string },
  options: { signal?: AbortSignal } = {}
): Promise<NewBearerSession> {
  const origin = parseServerUrl(serverUrl);
  const loopback =
    origin.hostname === 'localhost' ||
    origin.hostname === '[::1]' ||
    /^127\.\d+\.\d+\.\d+$/.test(origin.hostname);
  if (
    (origin.protocol !== 'https:' && !loopback) ||
    origin.pathname !== '/' ||
    origin.search ||
    origin.hash
  ) {
    throw new Error('Use an HTTPS server origin, or HTTP on loopback');
  }
  if (!input.login.trim() || !input.password) throw new Error('Enter a username and password');
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 15_000);
  try {
    const response = await fetch(new URL('/auth/login', origin), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ login: input.login.trim(), password: input.password }),
      redirect: 'error',
      credentials: 'omit',
      signal: options.signal
        ? AbortSignal.any([options.signal, controller.signal])
        : controller.signal
    });
    if (!response.ok) {
      await response.body?.cancel().catch(() => undefined);
      throw new PasswordLoginError(response.status);
    }
    const reader = response.body?.getReader();
    if (!reader) throw new PasswordLoginError();
    let text = '';
    let size = 0;
    const decoder = new TextDecoder();
    try {
      while (true) {
        const chunk = await reader.read();
        if (chunk.done) break;
        size += chunk.value.byteLength;
        if (size > 64 * 1024) throw new PasswordLoginError();
        text += decoder.decode(chunk.value, { stream: true });
      }
      text += decoder.decode();
    } finally {
      await reader.cancel().catch(() => undefined);
      reader.releaseLock();
    }
    const session = directBearerSession(JSON.parse(text));
    if (!session) throw new PasswordLoginError();
    return session;
  } catch (error) {
    if (error instanceof PasswordLoginError) throw error;
    throw new PasswordLoginError();
  } finally {
    clearTimeout(timer);
  }
}
