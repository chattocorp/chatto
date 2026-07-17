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
  generateServerId,
  serverRegistry,
  type RegisteredServer
} from '$lib/state/server/registry.svelte';
import { serverIdToSegment } from '$lib/navigation';
import { clearCachedUser } from './loadAuth';

const POPUP_WIDTH = 520;
const POPUP_HEIGHT = 600;
const POPUP_POLL_INTERVAL_MS = 250;
const POPUP_TIMEOUT_MS = 5 * 60 * 1000;

class OAuthPopupError extends Error {}

export async function startServerOAuthFlow(
  serverUrl: string,
  serverInfo: Pick<PublicServerInfo, 'name' | 'authorizeUrl' | 'iconUrl'>,
  beforeNavigate?: () => void
): Promise<void> {
  if (!serverInfo.authorizeUrl) {
    throw new Error('This server does not support OAuth sign-in.');
  }

  const verifier = generateCodeVerifier();
  const state = generateState();
  const redirectUri = `${window.location.origin}/servers/callback?mode=popup`;

  const flow = {
    verifier,
    state,
    remoteUrl: serverUrl,
    serverName: serverInfo.name,
    serverIconUrl: serverInfo.iconUrl ?? null
  };
  saveFlowState(flow);

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

  const responseChannel = createResponseChannel(state);
  if (responseChannel) {
    // The callback returns through BroadcastChannel, so the untrusted remote
    // page does not need a reference capable of navigating the main client.
    popup.opener = null;
  }

  const responsePromise = waitForPopupResponse(popup, state, responseChannel);

  try {
    const challenge = await generateCodeChallenge(verifier);
    const params = new URLSearchParams({
      response_type: 'code',
      redirect_uri: redirectUri,
      code_challenge: challenge,
      code_challenge_method: 'S256',
      state
    });

    popup.location.href = `${serverUrl}${serverInfo.authorizeUrl}?${params}`;

    const response = await responsePromise;
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
    loadAndClearFlowState();
    if (!popup.closed) popup.close();
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
  popup: Window,
  state: string,
  channel: BroadcastChannel | null
): Promise<OAuthPopupResponse> {
  return new Promise((resolveResponse, reject) => {
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
      if (!popup.closed) popup.close();
      resolveResponse(response);
    };

    const fail = (message: string) => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(new OAuthPopupError(message));
    };

    const handleWindowMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin || event.source !== popup) return;
      if (isOAuthPopupResponse(event.data)) settle(event.data);
    };

    window.addEventListener('message', handleWindowMessage);
    if (channel) {
      channel.onmessage = (event) => {
        if (isOAuthPopupResponse(event.data)) settle(event.data);
      };
    }

    const closePoll = window.setInterval(() => {
      if (popup.closed) fail('The sign-in window was closed before authorization completed.');
    }, POPUP_POLL_INTERVAL_MS);
    const timeout = window.setTimeout(
      () => fail('The server sign-in attempt timed out.'),
      POPUP_TIMEOUT_MS
    );
  });
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
    serverRegistry.updateServer(existing.id, {
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
  serverRegistry.addServer({
    id,
    url: flow.remoteUrl,
    name: flow.serverName || 'Chatto',
    iconUrl: flow.serverIconUrl,
    token: result.access_token,
    userId: result.user?.id ?? null,
    userLogin: result.user?.login ?? null,
    userDisplayName: result.user?.displayName ?? null,
    userAvatarUrl: result.user?.avatarUrl ?? null,
    reauthRequiredAt: null,
    addedAt: Date.now()
  });
  // Registration creates the retained store immediately, but discovery is
  // otherwise fire-and-forget. Complete capability discovery before routing
  // to the new server so the transport coordinator can deterministically
  // include its required projection stream on the first route transition.
  await serverRegistry.getStore(id).serverInfo.init();
  return id;
}

export async function startRemoteReauthentication(server: RegisteredServer): Promise<void> {
  const info = await getPublicServerInfo(server.url, { signal: AbortSignal.timeout(10000) });
  await startServerOAuthFlow(server.url, {
    name: info.name || server.name,
    authorizeUrl: info.authorizeUrl,
    iconUrl: info.iconUrl ?? server.iconUrl
  });
}

export function beginOriginReauthentication(): void {
  const path = window.location.pathname + window.location.search;
  sessionStorage.setItem('returnUrl', path);
  clearCachedUser();
  serverRegistry.clearOriginAuthentication();

  const redirect = resolve('/login') + '?' + new URLSearchParams({
    error: 'authentication_required',
    redirect: path
  });
  // eslint-disable-next-line svelte/no-navigation-without-resolve -- base route is resolved above; query parameters preserve the current app path
  void goto(redirect, { invalidateAll: true });
}
