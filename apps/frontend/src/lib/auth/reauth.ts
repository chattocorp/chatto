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
  type AuthorizationWindow
} from '$lib/oauth/authorizationWindow';
import {
  generateServerId,
  serverRegistry,
  type RegisteredServer
} from '$lib/state/server/registry.svelte';
import { serverIdToSegment } from '$lib/navigation';
import { clearCachedUser } from './loadAuth';
import { saveReturnUrl } from './returnNavigation';

const POPUP_WIDTH = 520;
const POPUP_HEIGHT = 600;
const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;

class OAuthPopupError extends Error {}

export function startServerOAuthFlow(
  serverUrl: string,
  serverInfo: Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>,
  beforeNavigate?: () => void,
  providerId?: string | null
): Promise<void> {
  return runServerOAuthFlow(
    serverUrl,
    Promise.resolve({ serverInfo, providerId: providerId ?? null }),
    beforeNavigate
  );
}

async function runServerOAuthFlow(
  serverUrl: string,
  details: Promise<{
    serverInfo: Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>;
    providerId: string | null;
  }>,
  beforeNavigate?: () => void
): Promise<void> {
  const verifier = generateCodeVerifier();
  const state = generateState();
  const redirectUri = `${window.location.origin}/servers/callback?mode=popup`;

  // Open synchronously from the user's click before hashing the PKCE verifier;
  // otherwise browsers may treat the secondary window as an unsolicited popup.
  const popup = window.open(
    'about:blank',
    `chatto-oauth-${state.slice(0, 12)}`,
    popupFeatures(window)
  );
  if (!popup) {
    loadAndClearFlowState();
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

  try {
    const { serverInfo, providerId } = await details;
    if (!serverInfo.authorizeUrl) {
      throw new Error('This server does not support OAuth sign-in.');
    }
    const flow = {
      verifier,
      state,
      remoteUrl: serverUrl,
      serverName: serverInfo.name,
      serverIconUrl: serverInfo.iconUrl ?? null
    };
    saveFlowState(flow);
    const challenge = await generateCodeChallenge(verifier);
    const params = new URLSearchParams({
      response_type: 'code',
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
    await goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }));
  } catch (err) {
    responseWait.cancel();
    loadAndClearFlowState();
    await closeAuthorizationWindow(authorizationWindow);
    throw err;
  }
}

function popupFeatures(owner: Window): string {
  const left = Math.max(0, Math.round(owner.screenX + (owner.outerWidth - POPUP_WIDTH) / 2));
  const top = Math.max(0, Math.round(owner.screenY + (owner.outerHeight - POPUP_HEIGHT) / 2));
  return `popup,width=${POPUP_WIDTH},height=${POPUP_HEIGHT},left=${left},top=${top}`;
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
      redirect_uri: redirectUri
    }),
    signal: AbortSignal.timeout(10000)
  });

  const result = await response.json();
  if (!response.ok) {
    throw new OAuthPopupError(
      result.error_description || result.error || 'Failed to exchange the authorization code.'
    );
  }
  if (!result.access_token) {
    throw new OAuthPopupError('The server did not return an access token.');
  }

  const existing = serverRegistry.servers.find(
    (server) => server.url.toLowerCase() === flow.remoteUrl.toLowerCase()
  );
  if (existing) {
    serverRegistry.updateRegistration(existing.id, {
      name: flow.serverName || existing.name,
      iconUrl: flow.serverIconUrl ?? existing.iconUrl
    });
    serverRegistry.replaceServerAuthentication(existing.id, {
      token: result.access_token,
      userId: result.user?.id ?? null,
      userLogin: result.user?.login ?? null,
      userDisplayName: result.user?.displayName ?? null,
      userAvatarUrl: result.user?.avatarUrl ?? null,
      reauthRequiredAt: null
    });
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
      addedAt: Date.now(),
      source: 'local'
    },
    {
      token: result.access_token,
      userId: result.user?.id ?? null,
      userLogin: result.user?.login ?? null,
      userDisplayName: result.user?.displayName ?? null,
      userAvatarUrl: result.user?.avatarUrl ?? null,
      reauthRequiredAt: null
    }
  );
  // Registration creates the retained store immediately, but discovery is
  // otherwise fire-and-forget. Complete server discovery before routing to the
  // new server so the transport coordinator can deterministically include its
  // required projection stream on the first route transition.
  await serverRegistry.getStore(id).serverInfo.init();
  return id;
}

export function startRemoteReauthentication(server: RegisteredServer): Promise<void> {
  const details = getPublicServerInfo(server.url, { signal: AbortSignal.timeout(10000) }).then(
    async (info) => {
      const { findAuthlingServerProvider } = await import('$lib/authling/serverProvider');
      const provider = await findAuthlingServerProvider(info.authProviders).catch(() => null);
      return {
        serverInfo: {
          name: info.name || server.name,
          authorizeUrl: info.authorizeUrl,
          iconUrl: info.iconUrl ?? server.iconUrl
        },
        providerId: provider?.id ?? null
      };
    }
  );
  return runServerOAuthFlow(server.url, details);
}

export function beginOriginReauthentication(): void {
  const path = window.location.pathname + window.location.search;
  saveReturnUrl(path);
  clearCachedUser();
  serverRegistry.clearOriginAuthentication();

  const redirect =
    resolve('/login') +
    '?' +
    new URLSearchParams({
      error: 'authentication_required',
      redirect: path
    });
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- base route is resolved above; query parameters preserve the current app path
  void goto(redirect, { invalidateAll: true });
}
