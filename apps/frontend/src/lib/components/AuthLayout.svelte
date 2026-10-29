<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { PublicServerInfo } from '@chatto/client/api/server';
  import { getAuthServerInfo } from './authServerInfo';
  import ServerBranding from './ServerBranding.svelte';

  const getServerInfo = getAuthServerInfo();

  let {
    children,
    title,
    titleOnlyWithSideBranding = false,
    compact = false,
    showBranding = true,
    centerContent = false,
    serverInfo: suppliedServerInfo
  }: {
    children: Snippet;
    /** Visible page heading, rendered as the form column's `h1` above the content. */
    title?: string;
    /** Keep the heading accessible, but show it only beside a separate branding pane. */
    titleOnlyWithSideBranding?: boolean;
    compact?: boolean;
    showBranding?: boolean;
    centerContent?: boolean;
    /** Public branding of a remote server; defaults to the origin auth context. */
    serverInfo?: Pick<
      PublicServerInfo,
      'name' | 'iconUrl' | 'bannerUrl' | 'description' | 'welcomeMessage'
    > | null;
  } = $props();

  const serverInfo = $derived(
    suppliedServerInfo === undefined ? getServerInfo() : suppliedServerInfo
  );
  const serverName = $derived(serverInfo?.name ?? 'Chatto');
  const iconUrl = $derived(serverInfo?.iconUrl ?? null);
  const bannerUrl = $derived(serverInfo?.bannerUrl ?? null);
  const description = $derived(serverInfo?.description ?? null);
  const welcomeMessage = $derived(serverInfo?.welcomeMessage ?? null);
  const hasBranding = $derived(bannerUrl || welcomeMessage || description);
</script>

<div class="flex min-h-0 flex-1 overflow-hidden">
  <!-- Left pane: server branding (hidden on mobile, hidden entirely if no branding content) -->
  {#if showBranding && hasBranding && !compact}
    <div class="hidden flex-1 overflow-y-auto border-e border-border bg-surface/30 p-8 md:block">
      <div data-page-reveal>
        <ServerBranding name={serverName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
      </div>
    </div>
  {/if}

  <!-- Right pane: form content -->
  <div
    class={[
      'flex flex-1 justify-center overflow-y-auto',
      centerContent ? 'items-center' : 'items-start',
      compact ? 'p-5 sm:p-6' : 'p-8'
    ]}
  >
    <div data-page-reveal class="w-full max-w-sm">
      <!-- Show compact branding header on mobile, or when no left pane -->
      {#if showBranding && compact}
        <div data-page-reveal class="mb-5">
          <ServerBranding name={serverName} {iconUrl} compact />
        </div>
      {:else if showBranding && !hasBranding}
        <div data-page-reveal class="mb-8">
          <ServerBranding name={serverName} {iconUrl} />
        </div>
      {:else if showBranding}
        <div data-page-reveal class="mb-8 md:hidden">
          <ServerBranding name={serverName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
        </div>
      {/if}

      {#if title}
        <h1
          class={[
            'text-center text-2xl font-bold',
            titleOnlyWithSideBranding
              ? showBranding && hasBranding && !compact
                ? 'sr-only md:not-sr-only md:mb-6'
                : 'sr-only'
              : compact
                ? 'mb-4'
                : 'mb-6'
          ]}
        >
          {title}
        </h1>
      {/if}

      {@render children()}
    </div>
  </div>
</div>
