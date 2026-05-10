<script lang="ts">
  import type { Snippet } from 'svelte';
  import InstanceBranding from './InstanceBranding.svelte';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';

  let { children }: { children: Snippet } = $props();

  const origin = $derived(instanceRegistry.originInstance);
  const originStore = $derived(origin ? instanceRegistry.tryGetStore(origin.id) : undefined);
  const instanceName = $derived(originStore?.instance.name ?? origin?.name ?? 'Chatto');
  const iconUrl = $derived(originStore?.instance.iconUrl ?? origin?.iconUrl ?? null);
  const bannerUrl = $derived(originStore?.instance.bannerUrl ?? null);
  const description = $derived(originStore?.instance.description ?? null);
  const welcomeMessage = $derived(originStore?.instance.welcomeMessage ?? null);
  const hasBranding = $derived(bannerUrl || welcomeMessage || description);
</script>

<div class="flex min-h-0 flex-1 overflow-hidden">
  <!-- Left pane: instance branding (hidden on mobile, hidden entirely if no branding content) -->
  {#if hasBranding}
  <div class="hidden flex-1 overflow-y-auto border-r border-border bg-surface/30 p-8 md:block">
    <div class="mx-auto max-w-md">
      <InstanceBranding name={instanceName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
    </div>
  </div>
  {/if}

  <!-- Right pane: form content -->
  <div class="flex flex-1 items-start justify-center overflow-y-auto p-8">
    <div class="w-full max-w-sm">
      <!-- Show compact branding header on mobile, or when no left pane -->
      {#if !hasBranding}
        <div class="mb-8">
          <InstanceBranding name={instanceName} {iconUrl} />
        </div>
      {:else}
        <div class="mb-8 md:hidden">
          <InstanceBranding name={instanceName} {iconUrl} {bannerUrl} {description} {welcomeMessage} />
        </div>
      {/if}

      {@render children()}
    </div>
  </div>
</div>
