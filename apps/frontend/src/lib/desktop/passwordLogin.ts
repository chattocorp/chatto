import { directBearerSession, persistedBearerSession } from '@chatto/client/auth/bearerSession';
import { invalidateAll } from '$app/navigation';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { serverRegistry } from '$lib/client';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';

/** Browser deployments keep the server-owned OAuth login page. */
export function hasPasswordLogin(): boolean {
  return typeof window !== 'undefined' && typeof window.chattoDesktop?.passwordLogin === 'function';
}

/** Install a host-issued session only while this view and its original registration remain current. */
export async function signInWithPassword(
  server: RegisteredServer,
  identifier: string,
  password: string,
  signal: AbortSignal
): Promise<void> {
  const login = window.chattoDesktop?.passwordLogin;
  if (!login) throw new Error('Desktop password sign-in is unavailable.');
  signal.throwIfAborted();
  const store = serverRegistry.tryGetStore(server.id);
  if (!store || !serverRegistry.getServer(server.id)) throw new Error('The server was removed.');
  const result = await login({ serverUrl: server.url, identifier, password });
  signal.throwIfAborted();
  if (serverRegistry.tryGetStore(server.id) !== store || !serverRegistry.getServer(server.id)) {
    throw new Error('The server session changed.');
  }
  const session =
    result &&
    typeof result === 'object' &&
    'ok' in result &&
    result.ok === true &&
    'session' in result
      ? directBearerSession(result.session)
      : null;
  if (!session) throw new Error('Sign-in failed.');
  serverRegistry.replaceServerAuthentication(server.id, {
    ...persistedBearerSession(session),
    userId: null,
    userLogin: null,
    userDisplayName: null,
    userAvatarUrl: null,
    reauthRequiredAt: null
  });
  resumePushRegistrationAfterAuthentication(server.id);
  await invalidateAll();
}
