<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { loadAndClearFlowState } from '$lib/oauth/pkce';
  import { serverRegistry, generateServerId } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  let status = $state<'loading' | 'error'>('loading');
  let errorMessage = $state('');

  onMount(async () => {
    const code = page.url.searchParams.get('code');
    const state = page.url.searchParams.get('state');
    const errorParam = page.url.searchParams.get('error');

    // Handle error responses from the authorization server
    if (errorParam) {
      status = 'error';
      errorMessage =
        page.url.searchParams.get('error_description') || `Authorization failed: ${errorParam}`;
      return;
    }

    if (!code) {
      status = 'error';
      errorMessage = 'No authorization code received.';
      return;
    }

    // Load the saved flow state (verifier, remote URL, etc.)
    const flow = loadAndClearFlowState();
    if (!flow) {
      status = 'error';
      errorMessage = 'OAuth flow state not found. The session may have expired. Please try again.';
      return;
    }

    // Validate state parameter (CSRF protection)
    if (state !== flow.state) {
      status = 'error';
      errorMessage = 'Invalid state parameter. This may be a CSRF attack.';
      return;
    }

    // Build the redirect_uri that we used in the authorize request
    const redirectUri = `${window.location.origin}/servers/callback`;

    try {
      // Exchange the authorization code for a bearer token
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
        status = 'error';
        errorMessage = result.error_description || result.error || 'Token exchange failed.';
        return;
      }

      if (!result.access_token) {
        status = 'error';
        errorMessage = 'Server did not return an access token.';
        return;
      }

      // Register or update the instance
      const existing = serverRegistry.servers.find(
        (i) => i.url.toLowerCase() === flow.remoteUrl.toLowerCase()
      );

      let serverId: string;
      if (existing) {
        serverRegistry.updateServer(existing.id, {
          name: flow.serverName ?? existing.name,
          iconUrl: flow.serverIconUrl ?? existing.iconUrl,
          token: result.access_token,
          userId: result.user?.id ?? null,
          userLogin: result.user?.login ?? null,
          userDisplayName: result.user?.displayName ?? null,
          userAvatarUrl: result.user?.avatarUrl ?? null
        });
        serverId = existing.id;
      } else {
        const id = generateServerId(
          flow.remoteUrl,
          serverRegistry.servers.map((i) => i.id)
        );

        serverRegistry.addServer({
          id,
          url: flow.remoteUrl,
          name: flow.serverName ?? 'Chatto',
          iconUrl: flow.serverIconUrl ?? null,
          token: result.access_token,
          userId: result.user?.id ?? null,
          userLogin: result.user?.login ?? null,
          userDisplayName: result.user?.displayName ?? null,
          userAvatarUrl: result.user?.avatarUrl ?? null,
          addedAt: Date.now()
        });
        serverId = id;
      }

      goto(
        resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) })
      );
    } catch (err) {
      status = 'error';
      if (err instanceof DOMException && err.name === 'AbortError') {
        errorMessage = 'Token exchange timed out. Please try again.';
      } else {
        errorMessage = err instanceof Error ? err.message : 'Token exchange failed.';
      }
    }
  });
</script>

<PageTitle title="Connecting..." />

<div class="flex min-h-0 flex-1 items-center justify-center p-8">
  {#if status === 'loading'}
    <div class="flex flex-col items-center gap-4">
      <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
      <p class="text-muted">Completing authentication...</p>
    </div>
  {:else}
    <div class="flex max-w-md flex-col items-center gap-4 text-center">
      <span class="iconify text-4xl text-danger uil--exclamation-triangle"></span>
      <p class="font-medium">Authentication Failed</p>
      <p class="text-sm text-muted">{errorMessage}</p>
      <a href={resolve('/')} class="btn btn-secondary cursor-pointer">Try Again</a>
    </div>
  {/if}
</div>
