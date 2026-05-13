<!--
@component

The "Add Server" dialog. Two stages in one modal:

1. URL — collects a hostname/URL and probes `/api/server` to confirm
   it's a Chatto server.
2. Preview — shows what was found (name, hostname, version) so the user
   can confirm before being bounced to the remote's OAuth login. On
   submit it kicks off the OAuth PKCE flow and redirects.

Internal naming stays "instance" (registry, file name, route ids) per
ADR-027 — only user-facing copy says "server".
-->
<script lang="ts">
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import {
    generateCodeChallenge,
    generateCodeVerifier,
    generateState,
    saveFlowState
  } from '$lib/oauth/pkce';
  import { TextInput } from '$lib/ui/form';
  import FormDialog from '$lib/ui/FormDialog.svelte';

  let {
    visible = $bindable(false),
    onclose
  }: {
    visible?: boolean;
    onclose: () => void;
  } = $props();

  type InstanceInfo = {
    name: string;
    version?: string;
    authMethods: string[];
    welcomeMessage?: string;
    authorizeUrl?: string;
    description?: string;
    iconUrl?: string | null;
    bannerUrl?: string | null;
  };

  type Stage = 'url' | 'preview';

  let stage = $state<Stage>('url');
  let serverUrl = $state('');
  let probedUrl = $state('');
  let probedInfo = $state<InstanceInfo | null>(null);
  let formError = $state('');
  let probing = $state(false);
  let connecting = $state(false);

  // Reset everything whenever the dialog is closed so reopening starts
  // fresh — the component is mounted persistently by its callers (sidebar,
  // instances page, login), so without this the previous URL and any prior
  // error/preview would still be present on the next open.
  $effect(() => {
    if (!visible) {
      stage = 'url';
      serverUrl = '';
      probedUrl = '';
      probedInfo = null;
      formError = '';
      probing = false;
      connecting = false;
    }
  });

  function normalizeUrl(url: string): string {
    let u = url.trim().replace(/\/+$/, '');
    if (!/^https?:\/\//i.test(u)) {
      u = 'https://' + u;
    }
    try {
      return new URL(u).origin;
    } catch {
      return u;
    }
  }

  function hasScheme(url: string): boolean {
    return /^https?:\/\//i.test(url.trim());
  }

  /**
   * Probe `${url}/api/server`. If the user typed a bare hostname (no
   * scheme), `normalizeUrl()` defaults to https — fall back to http on
   * connection failure so dev servers on plain http still work without
   * the user having to type the scheme.
   */
  async function probeWithFallback(
    rawInput: string,
    initialUrl: string
  ): Promise<{ url: string; response: Response }> {
    const fetchOnce = (u: string) =>
      fetch(`${u}/api/server`, { signal: AbortSignal.timeout(10000) });

    try {
      return { url: initialUrl, response: await fetchOnce(initialUrl) };
    } catch (err) {
      if (hasScheme(rawInput) || !initialUrl.startsWith('https://')) {
        throw err;
      }
      const httpUrl = 'http://' + initialUrl.slice('https://'.length);
      return { url: httpUrl, response: await fetchOnce(httpUrl) };
    }
  }

  function hostnameOf(url: string): string {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }

  async function handleProbe() {
    formError = '';

    const url = normalizeUrl(serverUrl);

    try {
      new URL(url);
    } catch {
      formError = 'Please enter a valid URL.';
      return;
    }

    const existing = serverRegistry.instances.find(
      (i) => i.url.toLowerCase() === url.toLowerCase()
    );
    if (existing && (existing.token || existing.userId)) {
      formError = 'This server is already connected.';
      return;
    }

    probing = true;

    try {
      const { url: probedFromUrl, response } = await probeWithFallback(serverUrl, url);

      if (!response.ok) {
        formError = `Server responded with ${response.status}. Is this a Chatto server?`;
        return;
      }

      const info = (await response.json()) as InstanceInfo;

      if (!info.name || !Array.isArray(info.authMethods)) {
        formError = 'This does not appear to be a Chatto server.';
        return;
      }

      if (!info.authorizeUrl) {
        formError = 'This server does not support OAuth authentication. It may need to be updated.';
        return;
      }

      probedUrl = probedFromUrl;
      probedInfo = info;
      stage = 'preview';
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        formError = 'Connection timed out. Check the URL and try again.';
      } else if (err instanceof TypeError) {
        formError = 'Could not connect. Check the URL and ensure CORS is configured.';
      } else {
        formError = err instanceof Error ? err.message : 'Failed to connect.';
      }
    } finally {
      probing = false;
    }
  }

  async function handleConnect() {
    if (!probedInfo || !probedInfo.authorizeUrl) return;

    formError = '';
    connecting = true;

    try {
      const verifier = generateCodeVerifier();
      const challenge = await generateCodeChallenge(verifier);
      const state = generateState();
      const redirectUri = `${window.location.origin}/servers/callback`;

      saveFlowState({
        verifier,
        state,
        remoteUrl: probedUrl,
        serverName: probedInfo.name,
        serverIconUrl: probedInfo.iconUrl ?? null
      });

      const params = new URLSearchParams({
        response_type: 'code',
        redirect_uri: redirectUri,
        code_challenge: challenge,
        code_challenge_method: 'S256',
        state
      });

      window.location.href = `${probedUrl}${probedInfo.authorizeUrl}?${params}`;
    } catch (err) {
      connecting = false;
      formError = err instanceof Error ? err.message : 'Failed to start sign-in.';
    }
  }

  // The button label is intentionally static. The server-supplied `name`
  // appears on the preview card (visually marked as "this is what the
  // server told us") but never inside our own action buttons — interpolating
  // it there would let a hostile server inject impersonation copy ("Sign
  // in to YourBank Login") into trusted UI chrome.
  const submitLabel = $derived(stage === 'preview' ? 'Sign in' : 'Connect');
  const submitIcon = $derived(stage === 'preview' ? 'iconify mdi--login' : 'iconify uil--link');
  const submitLoadingText = $derived(stage === 'preview' ? 'Redirecting…' : 'Connecting…');
  const loading = $derived(probing || connecting);
  const disabled = $derived(stage === 'url' && !serverUrl.trim());
</script>

<FormDialog
  bind:visible
  title="Add Server"
  {submitLabel}
  {submitIcon}
  {submitLoadingText}
  {loading}
  {disabled}
  error={formError}
  onsubmit={stage === 'url' ? handleProbe : handleConnect}
  {onclose}
>
  {#snippet description()}
    {#if stage === 'url'}
      Chatto is distributed — your client connects to each server directly.
      Enter a URL to add another.
    {:else if probedInfo}
      You're about to sign in on this server. You'll be sent to its
      sign-in page to authenticate.
    {/if}
  {/snippet}

  {#if stage === 'url'}
    <TextInput
      id="add-server-url"
      label="Server URL"
      bind:value={serverUrl}
      placeholder="chat.example.com"
      leadingIcon="uil--globe"
      disabled={probing}
      required
      autofocus
    />
  {:else if probedInfo}
    <div class="overflow-hidden rounded-lg border border-border bg-surface-100">
      {#if probedInfo.bannerUrl}
        <img
          src={probedInfo.bannerUrl}
          alt=""
          class="aspect-[1200/630] w-full object-cover"
        />
      {/if}
      <div class="flex items-start gap-3 p-4">
        <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg bg-surface-200">
          {#if probedInfo.iconUrl}
            <img src={probedInfo.iconUrl} alt="" class="h-12 w-12 rounded-lg object-cover" />
          {:else}
            <span class="iconify text-2xl text-muted uil--globe"></span>
          {/if}
        </div>
        <div class="min-w-0 flex-1">
          <div class="truncate text-lg font-semibold">{probedInfo.name}</div>
          <div class="truncate text-sm text-muted">{hostnameOf(probedUrl)}</div>
          {#if probedInfo.version}
            <div class="text-xs text-muted/70">Chatto v{probedInfo.version}</div>
          {/if}
          {#if probedInfo.description}
            <p class="mt-2 text-sm text-muted">{probedInfo.description}</p>
          {/if}
        </div>
      </div>
    </div>

    <button
      type="button"
      class="cursor-pointer text-left text-sm text-muted hover:text-text hover:underline"
      onclick={() => (stage = 'url')}
    >
      Wrong server? Edit URL
    </button>
  {/if}
</FormDialog>
