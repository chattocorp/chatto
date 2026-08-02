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
import { OAuthPopupError, openOAuthPopup } from '$lib/oauth/popup';
import {
  generateServerId,
  serverRegistry,
  type RegisteredServer
} from '$lib/state/server/registry.svelte';
import { serverIdToSegment } from '$lib/navigation';
import { clearCachedUser } from './loadAuth';
import { saveReturnUrl } from './returnNavigation';

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
  const popup = openOAuthPopup(state);

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

    popup.navigate(`${serverUrl}${serverInfo.authorizeUrl}?${params}`);

    const response = await popup.response;
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
    popup.close();
    throw err;
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
