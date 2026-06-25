<script lang="ts">
  import type { Snippet } from 'svelte';
  import { getAuthServerInfo } from './authServerInfo';
  import ServerBranding from './ServerBranding.svelte';

  let { children }: { children: Snippet } = $props();

  const getServerInfo = getAuthServerInfo();
  const serverInfo = $derived(getServerInfo());
  const serverName = $derived(serverInfo?.name ?? 'Chatto');
  const iconUrl = $derived(serverInfo?.iconUrl ?? null);
  const bannerUrl = $derived(serverInfo?.bannerUrl ?? null);
  const description = $derived(serverInfo?.description ?? null);
  const welcomeMessage = $derived(serverInfo?.welcomeMessage ?? null);
  const hasBranding = $derived(bannerUrl || welcomeMessage || description);
</script>

<div class="flex min-h-0 flex-1 overflow-hidden">
  <!-- Left pane: server branding (hidden on mobile, hidden entirely if no branding content) -->
  {#if hasBranding}
    <div class="hidden flex-1 overflow-y-auto border-r border-border bg-surface/30 p-8 md:block">
      <div class="mx-auto max-w-md">
        <ServerBranding name={serverName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
      </div>
    </div>
  {/if}

  <!-- Right pane: form content -->
  <div class="flex flex-1 items-start justify-center overflow-y-auto p-8">
    <div class="w-full max-w-sm">
      <!-- Show compact branding header on mobile, or when no left pane -->
      {#if !hasBranding}
        <div class="mb-8">
          <ServerBranding name={serverName} {iconUrl} />
        </div>
      {:else}
        <div class="mb-8 md:hidden">
          <ServerBranding name={serverName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
        </div>
      {/if}

      {@render children()}
    </div>
  </div>
</div>
