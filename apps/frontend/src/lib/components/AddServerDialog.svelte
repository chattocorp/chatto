<!--
@component

The "Add Server" dialog. Two stages in one modal:

1. URL — collects a hostname/URL and probes ServerDiscoveryService.GetServer
   to confirm it's a Chatto server.
2. Preview — shows what was found (name, hostname, version) so the user
   can confirm before opening the remote's OAuth login in a popup. On
   submit it kicks off the OAuth PKCE flow.

Internal naming stays "instance" (registry, file name, route ids) per
ADR-027 — only user-facing copy says "server".
-->
<script lang="ts">
  import { ConnectError } from '@connectrpc/connect';
  import { startServerOAuthFlow } from '$lib/auth/reauth';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getPublicServerInfo, type PublicServerInfo } from '$lib/api-client/server';
  import * as m from '$lib/i18n/messages';
  import { TextInput } from '$lib/ui/form';
  import FormDialog from '$lib/ui/FormDialog.svelte';

  let {
    visible = $bindable(false),
    onclose
  }: {
    visible?: boolean;
    onclose: () => void;
  } = $props();

  type Stage = 'url' | 'preview';

  let stage = $state<Stage>('url');
  let serverUrl = $state('');
  let probedUrl = $state('');
  let probedInfo = $state<PublicServerInfo | null>(null);
  let formError = $state('');
  let probing = $state(false);
  let connecting = $state(false);

  function reset() {
    stage = 'url';
    serverUrl = '';
    probedUrl = '';
    probedInfo = null;
    formError = '';
    probing = false;
    connecting = false;
  }

  function handleClose() {
    reset();
    onclose();
  }

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
   * Probe ServerDiscoveryService.GetServer. If the user typed a bare hostname
   * (no scheme), `normalizeUrl()` defaults to https — fall back to http on
   * connection failure so dev servers on plain http still work without the user
   * having to type the scheme.
   */
  async function probeWithFallback(
    rawInput: string,
    initialUrl: string
  ): Promise<{ url: string; info: PublicServerInfo }> {
    const fetchOnce = (u: string) => getPublicServerInfo(u, { signal: AbortSignal.timeout(10000) });

    try {
      return { url: initialUrl, info: await fetchOnce(initialUrl) };
    } catch (err) {
      if (hasScheme(rawInput) || !initialUrl.startsWith('https://')) {
        throw err;
      }
      const httpUrl = 'http://' + initialUrl.slice('https://'.length);
      return { url: httpUrl, info: await fetchOnce(httpUrl) };
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
      formError = m['add_server.invalid_url']();
      return;
    }

    const existing = serverRegistry.servers.find((i) => i.url.toLowerCase() === url.toLowerCase());
    if (existing && (existing.token || existing.userId)) {
      formError = m['add_server.already_connected']();
      return;
    }

    probing = true;

    try {
      const { url: probedFromUrl, info } = await probeWithFallback(serverUrl, url);

      if (!info.name) {
        formError = m['add_server.not_chatto_server']();
        return;
      }

      if (!info.authorizeUrl) {
        formError = m['add_server.oauth_unsupported']();
        return;
      }

      probedUrl = probedFromUrl;
      probedInfo = info;
      stage = 'preview';
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        formError = m['add_server.connection_timed_out']();
      } else if (err instanceof TypeError || err instanceof ConnectError) {
        formError = m['add_server.connection_failed']();
      } else {
        formError = err instanceof Error ? err.message : m['add_server.connect_failed']();
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
      // Close before navigation starts. The destination can wait for its
      // server projection, so waiting for goto() before dismissing this modal
      // would leave stale blocking UI over that hydration boundary.
      await startServerOAuthFlow(probedUrl, probedInfo, handleClose);
    } catch {
      connecting = false;
      formError = m['add_server.start_failed']();
    }
  }

  // The button label is intentionally static. The server-supplied `name`
  // appears on the preview card (visually marked as "this is what the
  // server told us") but never inside our own action buttons — interpolating
  // it there would let a hostile server inject impersonation copy ("Sign
  // in to YourBank Login") into trusted UI chrome.
  const submitLabel = $derived(
    stage === 'preview' ? m['add_server.sign_in']() : m['add_server.connect']()
  );
  const submitIcon = $derived(stage === 'preview' ? 'iconify mdi--login' : 'iconify uil--link');
  const submitLoadingText = $derived(
    stage === 'preview' ? m['add_server.redirecting']() : m['add_server.connecting']()
  );
  const loading = $derived(probing || connecting);
  const disabled = $derived(stage === 'url' && !serverUrl.trim());
</script>

<FormDialog
  bind:visible
  title={m['add_server.title']()}
  {submitLabel}
  {submitIcon}
  {submitLoadingText}
  {loading}
  {disabled}
  error={formError}
  onsubmit={stage === 'url' ? handleProbe : handleConnect}
  onclose={handleClose}
>
  {#snippet description()}
    {#if stage === 'url'}
      {m['add_server.description_url']()}
    {:else if probedInfo}
      {m['add_server.description_preview']()}
    {/if}
  {/snippet}

  {#if stage === 'url'}
    <TextInput
      id="add-server-url"
      label={m['add_server.url_label']()}
      bind:value={serverUrl}
      placeholder={m['add_server.url_placeholder']()}
      leadingIcon="uil--globe"
      disabled={probing}
      required
      autofocus
    />
  {:else if probedInfo}
    <div class="overflow-hidden rounded-lg border border-border bg-surface">
      {#if probedInfo.bannerUrl}
        <img
          src={probedInfo.bannerUrl}
          alt=""
          data-testid="server-preview-banner"
          class="h-24 w-full object-cover"
        />
      {/if}
      <div class="p-4">
        <div class="flex items-start gap-3">
          <div
            class={[
              'flex h-14 w-14 shrink-0 items-center justify-center rounded-lg border border-border bg-surface-emphasized',
              probedInfo.bannerUrl ? '-mt-9' : ''
            ]}
          >
            {#if probedInfo.iconUrl}
              <img src={probedInfo.iconUrl} alt="" class="h-full w-full rounded-lg object-cover" />
            {:else}
              <span class="iconify text-2xl text-muted uil--globe"></span>
            {/if}
          </div>
          <div class="min-w-0 flex-1">
            <div class="truncate text-lg font-semibold">{probedInfo.name}</div>
            <div class="flex flex-wrap items-baseline gap-x-2">
              <span class="truncate text-sm text-muted">{hostnameOf(probedUrl)}</span>
              {#if probedInfo.version}
                <span class="text-xs text-muted/70">Chatto v{probedInfo.version}</span>
              {/if}
            </div>
          </div>
        </div>
        {#if probedInfo.description}
          <p class="mt-3 line-clamp-2 text-sm text-muted">{probedInfo.description}</p>
        {/if}
      </div>
    </div>

    <button
      type="button"
      class="cursor-pointer text-left text-sm text-muted hover:text-text hover:underline"
      onclick={() => (stage = 'url')}
    >
      {m['add_server.edit_url']()}
    </button>
  {/if}
</FormDialog>
