import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { getPublicServerInfo, type PublicServerInfo } from '@chatto/client/api/server';
import {
  generateCodeChallenge,
  generateCodeVerifier,
  generateState,
  loadAndClearFlowState,
  saveFlowState
} from '$lib/oauth/pkce';
import {
  isOAuthPopupResponse,
  oauthPopupChannelName,
  type OAuthPopupResponse
} from '$lib/oauth/popup';
import {
  authorizationWindowFeatures,
  openAuthorizationWindow,
  type AuthorizationWindow
} from '$lib/oauth/authorizationWindow';
import { serverRegistry } from '$lib/client';
import { findServerByUrl } from '$lib/serverCatalogue';
import { getActiveServer } from '$lib/state/activeServer.svelte';
import type { RegisteredServer } from '@chatto/client/server/registry';
import { serverIdToSegment } from '$lib/navigation';
import { isLoopbackHostname } from '@chatto/client/util/runtimeOrigin';
import { LOOPBACK_OAUTH_CLIENT_ID } from '$lib/auth/loopbackClient';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';
import { saveReturnUrl } from './returnNavigation';
import { oauthBearerSession, persistedBearerSession } from '@chatto/client/auth/bearerSession';
import {
  authorizeNatively,
  hasNativeAuthorization,
  MOBILE_CALLBACK,
  MOBILE_CLIENT_ID
} from '$lib/desktop/nativeAuthorization';
import { m } from '$lib/i18n/messages';

const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;
const DESKTOP_CLIENT_ID = 'chatto://desktop';
const FRONTEND_CIMD_PATH = '/oauth/frontend-client-metadata.json';

class OAuthPopupError extends Error {}

/** Sign-in completed for a server that is no longer registered. */
class ServerNotRegisteredError extends Error {
  constructor() {
    super('The server is no longer registered.');
  }
}

/**
 * Sign in to the registered server at `serverUrl` and open it. Call this
 * synchronously from the user's action: the browser opens the sign-in window
 * before `serverInfo` settles. A rejected `serverInfo` closes the window and
 * rejects the returned promise with the same error. If the browser blocks the
 * window, the returned promise rejects with that error instead.
 */
async function runServerOAuthFlow(
  serverUrl: string,
  serverInfo: Promise<Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>>
): Promise<void> {
  const verifier = generateCodeVerifier();
  const state = generateState();
  if (hasNativeAuthorization()) {
    const info = await serverInfo;
    if (!info.authorizeUrl) throw new Error('This server does not support OAuth sign-in.');
    const challenge = await generateCodeChallenge(verifier);
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: MOBILE_CLIENT_ID,
      redirect_uri: MOBILE_CALLBACK,
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    });
    const response = await authorizeNatively(`${serverUrl}${info.authorizeUrl}?${params}`, state);
    if (response.error || !response.code) {
      throw new OAuthPopupError(
        response.errorDescription || response.error || 'Missing authorization code.'
      );
    }
    // The native session returns directly to this live operation. Its verifier
    // stays in memory; an app termination cancels the attempt instead of replaying it.
    const serverId = await completeServerOAuthFlow(
      {
        verifier,
        clientId: MOBILE_CLIENT_ID,
        remoteUrl: serverUrl,
        serverName: info.name,
        serverIconUrl: info.iconUrl ?? null
      },
      response.code,
      MOBILE_CALLBACK
    );
    await openSignedInServer(serverId);
    return;
  }
  const redirectUri = `${window.location.origin}/servers/callback?mode=popup`;
  const clientId = oauthClientIdForLocation(window.location, serverUrl);

  // Open synchronously from the user's click before hashing the PKCE verifier;
  // otherwise browsers may treat the secondary window as an unsolicited popup.
  const authorizationWindow: AuthorizationWindow | null = openAuthorizationWindow(
    `chatto-oauth-${state.slice(0, 12)}`,
    authorizationWindowFeatures(window)
  );
  if (!authorizationWindow) {
    loadAndClearFlowState();
    // The blocked window replaces any later server-data error.
    serverInfo.catch(() => {});
    throw new OAuthPopupError('The sign-in window could not be opened.');
  }

  const responseChannel = createResponseChannel(state);
  if (responseChannel) {
    // The callback returns through BroadcastChannel, so the untrusted remote
    // page does not need a reference capable of navigating the main client.
    authorizationWindow.detachOpener();
  }

  const responseWait = waitForPopupResponse(authorizationWindow, state, responseChannel);
  // The window can close while server data still loads. Observe that early
  // rejection here; the later await still receives it.
  responseWait.promise.catch(() => {});

  try {
    const info = await serverInfo;
    if (!info.authorizeUrl) {
      throw new Error('This server does not support OAuth sign-in.');
    }
    const flow = {
      verifier,
      state,
      remoteUrl: serverUrl,
      clientId,
      serverName: info.name,
      serverIconUrl: info.iconUrl ?? null
    };
    saveFlowState(flow);
    const challenge = await generateCodeChallenge(verifier);
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: clientId,
      redirect_uri: redirectUri,
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    });

    await authorizationWindow.navigate(`${serverUrl}${info.authorizeUrl}?${params}`);

    const response = await responseWait.promise;
    if (response.error) {
      throw new OAuthPopupError(response.errorDescription || response.error);
    }
    if (!response.code) {
      throw new OAuthPopupError('The server did not return an authorization code.');
    }

    const serverId = await completeServerOAuthFlow(flow, response.code, redirectUri);
    loadAndClearFlowState();
    await openSignedInServer(serverId);
  } catch (err) {
    responseWait.cancel();
    loadAndClearFlowState();
    await closeAuthorizationWindow(authorizationWindow);
    throw err;
  }
}

/**
 * Open a server after sign-in completes. When the current route already shows
 * the server, stay on it: the server layout replaces the signed-out view with
 * the server chrome, and the route keeps its deep link, for example a room.
 */
async function openSignedInServer(serverId: string): Promise<void> {
  if (getActiveServer() === serverId) return;
  await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }));
}

function createResponseChannel(state: string): BroadcastChannel | null {
  if (typeof BroadcastChannel === 'undefined') return null;
  return new BroadcastChannel(oauthPopupChannelName(state));
}

function waitForPopupResponse(
  authorizationWindow: AuthorizationWindow,
  state: string,
  channel: BroadcastChannel | null
): { promise: Promise<OAuthPopupResponse>; cancel: () => void } {
  let cancel = () => {};
  const promise = new Promise<OAuthPopupResponse>((resolveResponse, reject) => {
    let settled = false;

    const cleanup = () => {
      window.removeEventListener('message', handleWindowMessage);
      channel?.close();
      window.clearInterval(closePoll);
      window.clearTimeout(timeout);
    };

    const settle = (response: OAuthPopupResponse) => {
      if (settled || response.state !== state) return;
      settled = true;
      cleanup();
      void closeAuthorizationWindow(authorizationWindow);
      resolveResponse(response);
    };

    const fail = (message: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new OAuthPopupError(message));
    };

    cancel = () => {
      if (settled) return;
      settled = true;
      cleanup();
    };

    const handleWindowMessage = (event: MessageEvent) => {
      if (
        event.origin !== window.location.origin ||
        !authorizationWindow.messageSource ||
        event.source !== authorizationWindow.messageSource
      )
        return;
      if (isOAuthPopupResponse(event.data)) settle(event.data);
    };

    window.addEventListener('message', handleWindowMessage);
    if (channel) {
      channel.onmessage = (event) => {
        if (isOAuthPopupResponse(event.data)) settle(event.data);
      };
    }

    let polling = false;
    const closePoll = window.setInterval(async () => {
      if (polling || settled) return;
      polling = true;
      try {
        if (await authorizationWindow.isClosed()) {
          fail('The sign-in window was closed before authorization completed.');
        }
      } catch {
        fail('The sign-in window could not be inspected.');
      } finally {
        polling = false;
      }
    }, POPUP_POLL_INTERVAL_MS);
    const timeout = window.setTimeout(
      () => fail('The server sign-in attempt timed out.'),
      POPUP_TIMEOUT_MS
    );
  });
  return { promise, cancel };
}

async function closeAuthorizationWindow(authorizationWindow: AuthorizationWindow): Promise<void> {
  try {
    await authorizationWindow.close();
  } catch {
    // Closing is best-effort after the flow has already completed or failed.
  }
}

/**
 * Exchange an authorization code for a bearer session and store the session
 * for the registered server at `flow.remoteUrl`. Returns the server's ID.
 * Sign-in starts only for a registered server, so this never registers one.
 *
 * @throws Error when no registered server matches `flow.remoteUrl`, for
 *   example because the user removed it while sign-in was open.
 */
export async function completeServerOAuthFlow(
  flow: {
    remoteUrl: string;
    serverName: string;
    serverIconUrl: string | null;
    verifier: string;
    clientId: string;
  },
  code: string,
  redirectUri: string
): Promise<string> {
  // Check before the exchange, so that no session is created for a server
  // that this client no longer knows.
  if (!findServerByUrl(flow.remoteUrl)) throw new ServerNotRegisteredError();

  const response = await fetch(`${flow.remoteUrl}/oauth/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      grant_type: 'authorization_code',
      code,
      code_verifier: flow.verifier,
      redirect_uri: redirectUri,
      client_id: flow.clientId
    }),
    signal: AbortSignal.timeout(10000)
  });

  const result = await response.json();
  if (!response.ok) {
    throw new OAuthPopupError(
      result.error_description || result.error || m('auth.callback.token_exchange_failed')
    );
  }
  const credentials = oauthBearerSession(result, flow.clientId);
  if (!credentials) {
    throw new OAuthPopupError('The server did not return a renewable bearer session.');
  }

  const persistedCredentials = persistedBearerSession(credentials);

  // The user can remove the server while the exchange runs.
  const server = findServerByUrl(flow.remoteUrl);
  if (!server) throw new ServerNotRegisteredError();
  serverRegistry.updateRegistration(server.id, {
    name: flow.serverName || server.name,
    iconUrl: flow.serverIconUrl ?? server.iconUrl
  });
  serverRegistry.replaceServerAuthentication(server.id, {
    ...persistedCredentials,
    userId: result.user?.id ?? null,
    userLogin: result.user?.login ?? null,
    userDisplayName: result.user?.displayName ?? null,
    userAvatarUrl: result.user?.avatarUrl ?? null,
    reauthRequiredAt: null
  });
  resumePushRegistrationAfterAuthentication(server.id);
  // Complete discovery before routing to the server so the transport
  // coordinator can include its projection stream on the first route
  // transition.
  await serverRegistry.getStore(server.id).serverInfo.init();
  return server.id;
}

function isLoopbackServerUrl(serverUrl: string): boolean {
  try {
    return isLoopbackHostname(new URL(serverUrl).hostname);
  } catch {
    return false;
  }
}

/**
 * Choose the OAuth client identity that this frontend presents to `serverUrl`.
 * A loopback frontend uses the built-in loopback client only for a server that
 * is not local, because a local server can still fetch the frontend's CIMD
 * document. This keeps local servers without the built-in client working.
 */
export function oauthClientIdForLocation(
  location: Pick<Location, 'origin' | 'protocol' | 'host' | 'hostname'>,
  serverUrl: string
): string {
  if (location.protocol === 'chatto:' && location.host === 'desktop') {
    return DESKTOP_CLIENT_ID;
  }
  if (
    (location.protocol === 'http:' || location.protocol === 'https:') &&
    isLoopbackHostname(location.hostname) &&
    !isLoopbackServerUrl(serverUrl)
  ) {
    return LOOPBACK_OAUTH_CLIENT_ID;
  }
  return `${location.origin}${FRONTEND_CIMD_PATH}`;
}

/**
 * Sign in to a registered remote server in a separate window and open the
 * server when sign-in completes. Call this synchronously from the user's
 * action, so the browser allows the window.
 */
export function startRemoteReauthentication(server: RegisteredServer): Promise<void> {
  const serverInfo = getPublicServerInfo(server.url, { signal: AbortSignal.timeout(10000) }).then(
    (info) => ({
      name: info.name || server.name,
      authorizeUrl: info.authorizeUrl,
      iconUrl: info.iconUrl ?? server.iconUrl
    })
  );
  return runServerOAuthFlow(server.url, serverInfo);
}

export function beginOriginReauthentication(returnPath?: string): void {
  const path = returnPath ?? window.location.pathname + window.location.search;
  saveReturnUrl(path);
  serverRegistry.clearOriginAuthentication();

  const redirect =
    resolve('/login') +
    '?' +
    new URLSearchParams({
      redirect: path
    });
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- base route is resolved above; query parameters preserve the current app path
  void goto(redirect, { invalidateAll: true });
}
