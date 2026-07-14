<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import { loadAndClearFlowState } from '$lib/oauth/pkce';
  import { serverRegistry, generateServerId } from '$lib/state/server/registry.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import * as m from '$lib/i18n/messages';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { Button } from '$lib/ui/form';

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
        page.url.searchParams.get('error_description') ||
        m['auth.callback.authorization_failed']({ error: errorParam });
      return;
    }

    if (!code) {
      status = 'error';
      errorMessage = m['auth.callback.no_code']();
      return;
    }

    // Load the saved flow state (verifier, remote URL, etc.)
    const flow = loadAndClearFlowState();
    if (!flow) {
      status = 'error';
      errorMessage = m['auth.callback.missing_flow']();
      return;
    }

    // Validate state parameter (CSRF protection)
    if (state !== flow.state) {
      status = 'error';
      errorMessage = m['auth.callback.invalid_state']();
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
        errorMessage =
          result.error_description || result.error || m['auth.callback.token_exchange_failed']();
        return;
      }

      if (!result.access_token) {
        status = 'error';
        errorMessage = m['auth.callback.no_access_token']();
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
          reauthRequiredAt: null,
          addedAt: Date.now()
        });
        serverId = id;
      }

      goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(serverId) }));
    } catch (err) {
      status = 'error';
      if (err instanceof DOMException && err.name === 'AbortError') {
        errorMessage = m['auth.callback.token_exchange_timeout']();
      } else {
        errorMessage =
          err instanceof Error ? err.message : m['auth.callback.token_exchange_failed']();
      }
    }
  });
</script>

<PageTitle title={m['auth.callback.connecting_title']()} />

<div class="flex min-h-0 flex-1 items-center justify-center p-8">
  {#if status === 'loading'}
    <div class="flex flex-col items-center gap-4">
      <span class="iconify animate-spin text-3xl text-muted mdi--loading"></span>
      <p class="text-muted">{m['auth.callback.completing']()}</p>
    </div>
  {:else}
    <div class="flex max-w-md flex-col items-center gap-4 text-center">
      <span class="iconify text-4xl text-danger uil--exclamation-triangle"></span>
      <p class="font-medium">{m['auth.callback.failed_title']()}</p>
      <p class="text-sm text-muted">{errorMessage}</p>
      <Button href={resolve('/')} variant="secondary">{m['common.retry']()}</Button>
    </div>
  {/if}
</div>
