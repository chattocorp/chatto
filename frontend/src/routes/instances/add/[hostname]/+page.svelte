<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import {
    generateCodeVerifier,
    generateCodeChallenge,
    generateState,
    saveFlowState
  } from '$lib/oauth/pkce';
  import { FormError } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const hostname = page.params.hostname;

  let loading = $state(true);
  let error = $state('');

  async function probeUrl(url: string): Promise<Response> {
    return fetch(`${url}/api/instance`, { signal: AbortSignal.timeout(10000) });
  }

  onMount(async () => {
    // Try HTTPS first, fall back to HTTP (handles dev servers).
    let url = `https://${hostname}`;
    let response: Response;

    try {
      response = await probeUrl(url);
    } catch {
      // HTTPS failed — try HTTP (common in development)
      url = `http://${hostname}`;
      try {
        response = await probeUrl(url);
      } catch {
        error = 'Could not connect. Check the URL and ensure CORS is configured.';
        loading = false;
        return;
      }
    }

    try {
      if (!response.ok) {
        error = `Server responded with ${response.status}. Is this a Chatto instance?`;
        return;
      }

      const info = await response.json();

      if (!info.name || !Array.isArray(info.authMethods)) {
        error = 'This does not appear to be a Chatto instance.';
        return;
      }

      if (!info.authorizeUrl) {
        error = 'This instance does not support OAuth authentication. It may need to be updated.';
        return;
      }

      // Start OAuth PKCE flow immediately
      const verifier = generateCodeVerifier();
      const challenge = await generateCodeChallenge(verifier);
      const state = generateState();
      const redirectUri = `${window.location.origin}/instances/callback`;

      saveFlowState({
        verifier,
        state,
        remoteUrl: url,
        instanceName: info.name,
        instanceIconUrl: info.iconUrl ?? null
      });

      const params = new URLSearchParams({
        response_type: 'code',
        redirect_uri: redirectUri,
        code_challenge: challenge,
        code_challenge_method: 'S256',
        state
      });

      window.location.href = `${url}${info.authorizeUrl}?${params}`;
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        error = 'Connection timed out.';
      } else if (err instanceof TypeError) {
        error = 'Could not connect. Check the URL and ensure CORS is configured.';
      } else {
        error = err instanceof Error ? err.message : 'Failed to connect.';
      }
    } finally {
      loading = false;
    }
  });
</script>

<PageTitle title="Connecting to {hostname}" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Add Instance" showMobileNav />

  <div class="flex-1 overflow-y-auto p-6">
    <div class="mx-auto flex max-w-md flex-col gap-6">
      {#if loading}
        <div class="flex flex-col items-center gap-3 py-12">
          <span class="iconify animate-spin text-2xl text-muted mdi--loading"></span>
          <p class="text-sm text-muted">Connecting to {hostname}...</p>
        </div>
      {:else if error}
        <div class="flex flex-col items-center gap-4 py-8 text-center">
          <span class="iconify text-4xl text-danger uil--exclamation-triangle"></span>
          <FormError {error} />
          <a href={resolve('/instances')} class="btn btn-secondary cursor-pointer">Back</a>
        </div>
      {/if}
    </div>
  </div>
</div>
