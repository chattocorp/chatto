<script lang="ts">
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { m } from '$lib/i18n/messages';
  import { providerIcon } from '$lib/auth/providerIcon';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { Panel, Hint, LoadingFog } from '$lib/ui';

  const scope = useServerScope();
  const serverInfo = $derived(scope.store.serverInfo);
  const registrationProviders = $derived(
    serverInfo.authProviders.filter((provider) => provider.autoProvision !== false)
  );
</script>

<!-- @component Displays signup methods configured by the server's hosting operator. -->
<Panel title={m('admin.registration.methods')}>
  <div class="flex flex-col gap-3 text-sm">
    {#if serverInfo.loading}
      <LoadingFog class="h-6 w-full" />
    {:else if serverInfo.error}
      <Hint tone="danger">{errorMessage(serverInfo.error)}</Hint>
    {:else if serverInfo.directRegistrationEnabled || registrationProviders.length > 0}
      <ul
        role="list"
        class="flex min-w-0 flex-wrap gap-3"
        aria-label={m('admin.registration.methods')}
      >
        {#if serverInfo.directRegistrationEnabled}
          <li class="flex max-w-full min-w-0 items-center gap-3 rounded border border-border p-3">
            <span
              aria-hidden="true"
              class="iconify icon-[uil--envelope] shrink-0 text-lg text-muted"
            ></span>
            <span class="min-w-0 font-medium wrap-anywhere"
              >{m('admin.registration.email_password')}</span
            >
          </li>
        {/if}
        {#each registrationProviders as provider (provider.id)}
          <li class="flex max-w-full min-w-0 items-center gap-3 rounded border border-border p-3">
            <span
              aria-hidden="true"
              class={['iconify shrink-0 text-lg text-muted', providerIcon(provider.type)]}
            ></span>
            <span class="min-w-0 font-medium wrap-anywhere">{provider.label}</span>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="text-muted">{m('admin.registration.no_methods')}</p>
    {/if}
    <p class="text-muted">{m('admin.registration.methods_configuration')}</p>
  </div>
</Panel>
