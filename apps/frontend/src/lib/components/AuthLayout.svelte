<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { PublicServerInfo } from '@chatto/client/api/server';
  import { getAuthServerInfo } from './authServerInfo';
  import ServerBranding from './ServerBranding.svelte';
  import { ScrollFader } from '$lib/ui';

  const getServerInfo = getAuthServerInfo();

  let {
    children,
    title,
    titleOnlyWithSideBranding = false,
    compact = false,
    showBranding = true,
    serverInfo: suppliedServerInfo
  }: {
    children: Snippet;
    /** Visible page heading, rendered as the form column's `h1` above the content. */
    title?: string;
    /** Keep the heading accessible, but show it only beside a separate branding pane. */
    titleOnlyWithSideBranding?: boolean;
    /**
     * Compact layout for pages where something else names the server: the
     * window bar of a sign-in window, or a title such as "Join {server}". The
     * page shows only the server banner and description.
     */
    compact?: boolean;
    showBranding?: boolean;
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
  /**
   * Without a banner, description, or welcome message, the server name sits
   * directly above the title. The two then read as one heading group: the
   * name leads, and the title is its quieter second line.
   */
  const titleFollowsName = $derived(showBranding && !hasBranding && !compact);
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

  <!-- Right pane: form content, centred in the pane. The automatic margins
       collapse when the content is taller than the pane, so it then starts at
       the top and scrolls with edge fades. Sign-in windows stay top-aligned
       below their window bar. -->
  <ScrollFader top bottom mask keyboardFocusable={false} class="min-w-0">
    <div class={['flex min-h-full flex-1 justify-center', compact ? 'p-5 sm:p-6' : 'p-8']}>
      <div data-page-reveal class={['w-full max-w-sm', !compact && 'my-auto']}>
        <!-- Show compact branding header on mobile, or when no left pane -->
        {#if showBranding && compact}
          {#if bannerUrl || description}
            <div data-page-reveal class="mb-5">
              <ServerBranding name={serverName} {iconUrl} {bannerUrl} {description} compact />
            </div>
          {/if}
        {:else if showBranding && !hasBranding}
          <div data-page-reveal class="mb-1">
            <ServerBranding name={serverName} {iconUrl} />
          </div>
        {:else if showBranding}
          <div data-page-reveal class="mb-8 md:hidden">
            <ServerBranding
              name={serverName}
              {iconUrl}
              {bannerUrl}
              {description}
              {welcomeMessage}
            />
          </div>
        {/if}

        {#if title}
          <h1
            class={[
              'text-center text-balance',
              titleFollowsName ? 'text-lg font-medium text-muted' : 'text-xl font-semibold',
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
  </ScrollFader>
</div>
