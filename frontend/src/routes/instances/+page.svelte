<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { PaneHeader, EmptyState } from '$lib/ui';
  import { Button } from '$lib/ui/form';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  // Redirect to login if origin exists but user isn't authenticated
  const origin = $derived(instanceRegistry.originInstance);
  const originAuthenticated = $derived(origin ? instanceRegistry.isAuthenticated(origin.id) : false);
  $effect(() => {
    if (origin && !originAuthenticated) {
      goto(resolve('/login'), { replaceState: true });
    }
  });

  let confirmingDisconnect = $state<string | null>(null);

  function getHostname(url: string): string {
    try {
      return new URL(url).hostname;
    } catch {
      return url;
    }
  }

  function formatDate(epochMs: number): string {
    return new Date(epochMs).toLocaleDateString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric'
    });
  }

  async function disconnect(instanceId: string) {
    instanceRegistry.removeInstance(instanceId);
    confirmingDisconnect = null;
  }

  let confirmInstance = $derived(
    confirmingDisconnect
      ? instanceRegistry.getInstance(confirmingDisconnect)
      : undefined
  );
</script>

<PageTitle title="Instances" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader title="Connected Instances" subtitle="Manage your Chatto instance connections" showMobileNav>
    {#snippet actions()}
      <a href={resolve('/instances/add')} class="btn btn-secondary btn-sm cursor-pointer">
        <span class="iconify uil--plus mr-1"></span>
        Add Instance
      </a>
    {/snippet}
  </PaneHeader>

  <div class="flex-1 overflow-y-auto">
    <div class="flex flex-col divide-y divide-border">
      {#each instanceRegistry.instances as instance (instance.id)}
        {@const hostname = getHostname(instance.url)}
        {@const isOrigin = instanceRegistry.isOriginInstance(instance.id)}
        {@const store = instanceRegistry.getStore(instance.id)}
        {@const authenticated = store.isAuthenticated}

        <div class="flex items-center gap-4 px-6 py-4" data-testid="instance-row">
          <!-- Instance icon -->
          <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-surface-200">
            {#if instance.iconUrl}
              <img src={instance.iconUrl} alt="" class="h-10 w-10 rounded-lg object-cover" />
            {:else}
              <span class="iconify text-lg text-muted uil--globe"></span>
            {/if}
          </div>

          <!-- Instance info -->
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-medium">{store.instance.name || hostname}</span>
            </div>

            <div class="flex items-center gap-3 text-sm text-muted">
              <span>{hostname}</span>
              <span>·</span>
              {#if authenticated}
                <span class="text-success">Connected</span>
                {#if instance.userLogin}
                  <span>·</span>
                  <span>@{instance.userLogin}</span>
                {/if}
              {:else}
                <span>Not authenticated</span>
              {/if}
            </div>

            <div class="text-xs text-muted/60">
              Added {formatDate(instance.addedAt)}
            </div>
          </div>

          <!-- Actions -->
          {#if !isOrigin}
            <div class="shrink-0">
              <Button variant="secondary" size="sm" onclick={() => (confirmingDisconnect = instance.id)}>
                Disconnect
              </Button>
            </div>
          {/if}
        </div>
      {/each}
    </div>

    {#if instanceRegistry.instances.length === 0}
      <EmptyState icon="uil--globe" title="No instances connected">
        Add a Chatto instance to get started.
      </EmptyState>
    {/if}
  </div>
</div>

{#if confirmingDisconnect && confirmInstance}
  <ConfirmDialog
    title="Disconnect Instance"
    actionLabel="Disconnect"
    actionIcon="iconify uil--link-broken"
    onconfirm={() => disconnect(confirmingDisconnect!)}
    onclose={() => (confirmingDisconnect = null)}
  >
    Disconnect from <strong>{confirmInstance.name || getHostname(confirmInstance.url)}</strong>?
    This only removes it from your sidebar. Your account and memberships on the instance are not affected.
  </ConfirmDialog>
{/if}
