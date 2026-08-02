<script lang="ts">
  import { onMount } from 'svelte';
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';

  type AccountDataSyncAPI = typeof import('./sync.svelte').accountDataSync;
  let sync = $state<AccountDataSyncAPI | null>(null);
  let syncModule: Promise<typeof import('./sync.svelte')> | null = null;

  function loadSync() {
    syncModule ??= import('./sync.svelte');
    return syncModule;
  }

  onMount(() => {
    void loadSync().then(({ accountDataSync }) => {
      sync = accountDataSync;
      return sync.initialize();
    });
  });

  const status = $derived(sync?.status ?? 'disconnected');
  const title = $derived.by(() => {
    switch (status) {
      case 'connecting':
        return m['chat.server_gutter.account_data_connecting']();
      case 'connected':
        return m['chat.server_gutter.account_data_connected']({
          provider: sync?.providerLabel ?? 'Authling'
        });
      case 'error':
        return m['chat.server_gutter.account_data_error']();
      default:
        return m['chat.server_gutter.account_data_connect']();
    }
  });

  async function connect() {
    const { accountDataSync } = await loadSync();
    sync = accountDataSync;
    await sync.connect();
    if (sync.status === 'connected') {
      toast.success(m['chat.server_gutter.account_data_connected_toast']());
    } else if (sync.status === 'error') {
      toast.error(m['chat.server_gutter.account_data_error']());
    }
  }
</script>

<button
  type="button"
  onclick={connect}
  disabled={status === 'connecting' || status === 'connected'}
  {title}
  aria-label={title}
  data-state={status}
  class={[
    'server-gutter-item cursor-pointer disabled:cursor-default',
    status === 'connected' && 'text-success',
    status === 'error' && 'text-danger'
  ]}
>
  {#if status === 'connecting'}
    <span class="iconify animate-spin mdi--loading"></span>
  {:else if status === 'connected'}
    <span class="iconify uil--cloud-check"></span>
  {:else if status === 'error'}
    <span class="iconify uil--cloud-times"></span>
  {:else}
    <span class="iconify uil--cloud-upload"></span>
  {/if}
</button>
