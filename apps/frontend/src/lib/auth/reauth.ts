import { goto } from '$app/navigation';
import { resolve } from '$app/paths';
import { getPublicServerInfo, type PublicServerInfo } from '$lib/api-client/server';
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
  browserAuthorizationWindow,
  authorizationWindowFeatures,
  type AuthorizationWindow
} from '$lib/oauth/authorizationWindow';
import {
  generateServerId,
  serverRegistry,
  type RegisteredServer
} from '$lib/state/server/registry.svelte';
import { serverIdToSegment } from '$lib/navigation';
import { resumePushRegistrationAfterAuthentication } from '$lib/notifications/pushRegistrationCoordinator';
import { saveReturnUrl } from './returnNavigation';
import { oauthBearerSession, persistedBearerSession } from './bearerSession';
import {
  authorizeNatively,
  hasNativeAuthorization,
  MOBILE_CALLBACK,
  MOBILE_CLIENT_ID
} from '$lib/desktop/nativeAuthorization';

const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;
const DESKTOP_CLIENT_ID = 'chatto://desktop';
const FRONTEND_CIMD_PATH = '/oauth/frontend-client-metadata.json';

class OAuthPopupError extends Error {}

/** How a completed sign-in navigates to the server. */
export type ServerOAuthFlowOptions = {
  /**
   * Called when sign-in completes, just before navigation. Return `true` to
   * replace the current history entry instead of adding one. A history-backed
   * dialog uses this so that Back does not reopen it, and checks at that time
   * that the dialog is still open.
   */
  replaceHistory?: () => boolean;
};

/** Start sign-in with public server data that the caller already loaded. */
export function startServerOAuthFlow(
  serverUrl: string,
  serverInfo: Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>,
  {
    beforeNavigate,
    providerId,
    ...options
  }: ServerOAuthFlowOptions & {
    /** Called after sign-in completes and before navigation. */
    beforeNavigate?: () => void;
    /** Server-configured login provider that the authorization page starts. */
    providerId?: string | null;
  } = {}
): Promise<void> {
  return runServerOAuthFlow(
    serverUrl,
    Promise.resolve({ serverInfo, providerId: providerId ?? null }),
    beforeNavigate,
    options
  );
}

/**
 * Start sign-in while the server's current public data still loads. Call this
 * synchronously from the user's action: the browser opens the sign-in window
 * before `serverInfo` settles. A rejected `serverInfo` closes the window and
 * rejects the returned promise with the same error. If the browser blocks the
 * window, the returned promise rejects with that error instead.
 */
export function startServerOAuthFlowWhenReady(
  serverUrl: string,
  serverInfo: Promise<Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>>,
  options: ServerOAuthFlowOptions = {}
): Promise<void> {
  return runServerOAuthFlow(
    serverUrl,
    serverInfo.then((info) => ({ serverInfo: info, providerId: null })),
    undefined,
    options
  );
}

async function runServerOAuthFlow(
  serverUrl: string,
  details: Promise<{
    serverInfo: Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>;
    providerId: string | null;
  }>,
  beforeNavigate?: () => void,
  options: ServerOAuthFlowOptions = {}
): Promise<void> {
  const verifier = generateCodeVerifier();
  const state = generateState();
  if (hasNativeAuthorization()) {
    const { serverInfo, providerId } = await details;
    if (!serverInfo.authorizeUrl) throw new Error('This server does not support OAuth sign-in.');
    const challenge = await generateCodeChallenge(verifier);
    const params = new URLSearchParams({
      response_type: 'code',
      client_id: MOBILE_CLIENT_ID,
      redirect_uri: MOBILE_CALLBACK,
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    });
    if (providerId) params.set('provider_id', providerId);
    const response = await authorizeNatively(
      `${serverUrl}${serverInfo.authorizeUrl}?${params}`,
      state
    );
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
        serverName: serverInfo.name,
        serverIconUrl: serverInfo.iconUrl ?? null
      },
      response.code,
      MOBILE_CALLBACK
    );
    beforeNavigate?.();
    await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }), {
      replaceState: options.replaceHistory?.() ?? false
    });
    return;
  }
  const redirectUri = `${window.location.origin}/servers/callback?mode=popup`;
  const clientId = oauthClientIdForLocation(window.location);

  // Open synchronously from the user's click before hashing the PKCE verifier;
  // otherwise browsers may treat the secondary window as an unsolicited popup.
  const popup = window.open(
    'about:blank',
    `chatto-oauth-${state.slice(0, 12)}`,
    authorizationWindowFeatures(window)
  );
  if (!popup) {
    loadAndClearFlowState();
    // The blocked window replaces any later server-data error.
    details.catch(() => {});
    throw new OAuthPopupError('The sign-in window could not be opened.');
  }
  const authorizationWindow: AuthorizationWindow = browserAuthorizationWindow(popup);

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
    const { serverInfo, providerId } = await details;
    if (!serverInfo.authorizeUrl) {
      throw new Error('This server does not support OAuth sign-in.');
    }
    const flow = {
      verifier,
      state,
      remoteUrl: serverUrl,
      clientId,
      serverName: serverInfo.name,
      serverIconUrl: serverInfo.iconUrl ?? null
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
    if (providerId) params.set('provider_id', providerId);

    await authorizationWindow.navigate(`${serverUrl}${serverInfo.authorizeUrl}?${params}`);

    const response = await responseWait.promise;
    if (response.error) {
      throw new OAuthPopupError(response.errorDescription || response.error);
    }
    if (!response.code) {
      throw new OAuthPopupError('The server did not return an authorization code.');
    }

    const serverId = await completeServerOAuthFlow(flow, response.code, redirectUri);
    loadAndClearFlowState();
    beforeNavigate?.();
    await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }), {
      replaceState: options.replaceHistory?.() ?? false
    });
  } catch (err) {
    responseWait.cancel();
    loadAndClearFlowState();
    await closeAuthorizationWindow(authorizationWindow);
    throw err;
  }
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
      result.error_description || result.error || 'Failed to exchange the authorization code.'
    );
  }
  const credentials = oauthBearerSession(result, flow.clientId);
  if (!credentials) {
    throw new OAuthPopupError('The server did not return a renewable bearer session.');
  }

  const persistedCredentials = persistedBearerSession(credentials);

  const existing = serverRegistry.servers.find(
    (server) => server.url.toLowerCase() === flow.remoteUrl.toLowerCase()
  );
  if (existing) {
    serverRegistry.updateRegistration(existing.id, {
      name: flow.serverName || existing.name,
      iconUrl: flow.serverIconUrl ?? existing.iconUrl
    });
    serverRegistry.replaceServerAuthentication(existing.id, {
      ...persistedCredentials,
      userId: result.user?.id ?? null,
      userLogin: result.user?.login ?? null,
      userDisplayName: result.user?.displayName ?? null,
      userAvatarUrl: result.user?.avatarUrl ?? null,
      reauthRequiredAt: null
    });
    resumePushRegistrationAfterAuthentication(existing.id);
    await serverRegistry.getStore(existing.id).serverInfo.init();
    return existing.id;
  }

  const id = generateServerId(
    flow.remoteUrl,
    serverRegistry.servers.map((server) => server.id)
  );
  serverRegistry.addServer(
    {
      id,
      url: flow.remoteUrl,
      name: flow.serverName || 'Chatto',
      iconUrl: flow.serverIconUrl,
      addedAt: Date.now()
    },
    {
      ...persistedCredentials,
      userId: result.user?.id ?? null,
      userLogin: result.user?.login ?? null,
      userDisplayName: result.user?.displayName ?? null,
      userAvatarUrl: result.user?.avatarUrl ?? null,
      reauthRequiredAt: null
    }
  );
  resumePushRegistrationAfterAuthentication(id);
  // Registration creates the retained store immediately, but discovery is
  // otherwise fire-and-forget. Complete server discovery before routing to the
  // new server so the transport coordinator can deterministically include its
  // required projection stream on the first route transition.
  await serverRegistry.getStore(id).serverInfo.init();
  return id;
}

export function oauthClientIdForLocation(
  location: Pick<Location, 'origin' | 'protocol' | 'host'>
): string {
  if (location.protocol === 'chatto:' && location.host === 'desktop') {
    return DESKTOP_CLIENT_ID;
  }
  return `${location.origin}${FRONTEND_CIMD_PATH}`;
}

export function startRemoteReauthentication(
  server: RegisteredServer,
  options: ServerOAuthFlowOptions = {}
): Promise<void> {
  const details = getPublicServerInfo(server.url, { signal: AbortSignal.timeout(10000) }).then(
    (info) => ({
      serverInfo: {
        name: info.name || server.name,
        authorizeUrl: info.authorizeUrl,
        iconUrl: info.iconUrl ?? server.iconUrl
      },
      providerId: null
    })
  );
  return runServerOAuthFlow(server.url, details, undefined, options);
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
