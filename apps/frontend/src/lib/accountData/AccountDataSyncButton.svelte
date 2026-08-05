<script lang="ts">
  import { onMount } from 'svelte';
  import { getClientConfiguration } from '$lib/clientConfig';
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';

  type AccountDataSyncAPI = Pick<
    typeof import('./sync.svelte').accountDataSync,
    'status' | 'providerLabel' | 'accountId' | 'initialize' | 'connect'
  >;
  type SyncModule = { accountDataSync: AccountDataSyncAPI };

  let {
    getConfiguration = getClientConfiguration,
    loadSyncModule = () => import('./sync.svelte')
  }: {
    getConfiguration?: typeof getClientConfiguration;
    loadSyncModule?: () => Promise<SyncModule>;
  } = $props();

  let sync = $state<AccountDataSyncAPI | null>(null);
  let available = $state(false);
  let syncModule: Promise<SyncModule> | null = null;

  function loadSync() {
    syncModule ??= loadSyncModule();
    return syncModule;
  }

  onMount(() => {
    void getConfiguration()
      .then((configuration) => {
        if (!configuration.authling) return;
        available = true;
        return loadSync().then(({ accountDataSync }) => {
          sync = accountDataSync;
          return sync.initialize();
        });
      })
      .catch((error) => console.error('[account-data] invalid client configuration', error));
  });

  const status = $derived(sync?.status ?? 'disconnected');
  const title = $derived.by(() => {
    switch (status) {
      case 'connecting':
        return m('chat.server_gutter.account_data_connecting');
      case 'connected':
        return m('chat.server_gutter.account_data_connected', {
          provider: sync?.providerLabel ?? 'Authling'
        });
      case 'error':
        return m('chat.server_gutter.account_data_error');
      default:
        return m('chat.server_gutter.account_data_connect');
    }
  });
  const accessibleTitle = $derived(sync?.accountId ? `${title} · ${sync.accountId}` : title);

  async function connect() {
    const { accountDataSync } = await loadSync();
    sync = accountDataSync;
    await sync.connect();
    if (sync.status === 'connected') {
      toast.success(m('chat.server_gutter.account_data_connected_toast'));
    } else if (sync.status === 'error') {
      toast.error(m('chat.server_gutter.account_data_error'));
    }
  }
</script>

{#if available}
  <button
    type="button"
    onclick={connect}
    disabled={status === 'connecting' || status === 'connected'}
    title={accessibleTitle}
    aria-label={accessibleTitle}
    data-state={status}
    class={[
      'app-header-icon cursor-pointer disabled:cursor-default',
      status === 'connected' && 'text-success',
      status === 'error' && 'text-danger'
    ]}
  >
    <span class={['iconify icon-[uil--sync] text-lg', status === 'connecting' && 'animate-spin']}
    ></span>
  </button>
{/if}
