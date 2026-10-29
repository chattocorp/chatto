<script lang="ts">
  import {
    createRegistrationSettingsAPI,
    type RegistrationAdmission
  } from '$lib/api/registrationSettings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery, createMutation, queryClient } from '$lib/query/client';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import { Panel, Hint, LoadingFog, ChoiceRow, Pill } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  const scope = useServerScope();
  const session = createSessionGuard(scope);
  const key = adminQueryKeys.registrationSettings(scope.serverId, scope.connection);
  const settings = createQuery(() => ({
    queryKey: key,
    queryFn: ({ signal }) => scope.connection.getAPI(createRegistrationSettingsAPI).get({ signal })
  }));
  const serverInfo = $derived(scope.store.serverInfo);
  const registrationProviders = $derived(
    serverInfo.authProviders.filter((provider) => provider.autoProvision !== false)
  );
  const mutation = createMutation(() => ({
    mutationFn: (variables: {
      admission: RegistrationAdmission;
      snapshot: ReturnType<typeof session.snapshot>;
    }) =>
      variables.snapshot.connection
        .getAPI(createRegistrationSettingsAPI)
        .update(variables.admission),
    onSuccess: (value, variables) => {
      if (!session.isCurrent(variables.snapshot)) return;
      queryClient.setQueryData(key, value);
      toast.success(m('admin.security.settings_saved'));
    },
    onError: (error, variables) => {
      if (session.isCurrent(variables.snapshot)) toastError(error);
    }
  }));

  function save(admission: RegistrationAdmission) {
    if (admission === settings.data || mutation.isPending) return;
    mutation.mutate({
      admission,
      snapshot: session.snapshot()
    });
  }
</script>

<!-- @component Controls admission for new accounts on this server. -->
<Panel title={m('admin.registration.title')}>
  {#if settings.isPending}
    <LoadingFog class="h-24 w-full" />
  {:else if settings.error}
    <Hint tone="danger">{errorMessage(settings.error)}</Hint>
  {:else}
    <div role="radiogroup" aria-label={m('admin.registration.title')} class="flex flex-col gap-2">
      <ChoiceRow
        label={m('admin.registration.open')}
        description={m('admin.registration.open_description')}
        selected={settings.data === 'open'}
        disabled={mutation.isPending}
        onclick={() => save('open')}
      />
      <ChoiceRow
        label={m('admin.registration.invite_only')}
        description={m('admin.registration.invite_only_description')}
        selected={settings.data === 'invite_only'}
        disabled={mutation.isPending}
        onclick={() => save('invite_only')}
      />
    </div>
    <div
      class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-border pt-4 text-sm"
    >
      <p class="text-muted">{m('admin.registration.methods')}</p>
      {#if serverInfo.loading}
        <LoadingFog class="mt-2 h-6 w-full" />
      {:else if serverInfo.error}
        <Hint tone="danger">{errorMessage(serverInfo.error)}</Hint>
      {:else if serverInfo.directRegistrationEnabled || registrationProviders.length > 0}
        <ul
          role="list"
          class="flex min-w-0 flex-wrap gap-2"
          aria-label={m('admin.registration.methods')}
        >
          {#if serverInfo.directRegistrationEnabled}
            <li class="max-w-full">
              <Pill tone="default" paddingClass="px-2.5 py-1.5" class="max-w-full">
                <span class="flex items-center gap-2">
                  <span
                    aria-hidden="true"
                    class="iconify icon-[uil--envelope] shrink-0 text-base text-muted"
                  ></span>
                  <span class="min-w-0 wrap-anywhere">{m('admin.registration.email_password')}</span
                  >
                </span>
              </Pill>
            </li>
          {/if}
          {#each registrationProviders as provider (provider.id)}
            <li class="max-w-full">
              <Pill tone="default" paddingClass="px-2.5 py-1.5" class="max-w-full">
                <span class="flex items-center gap-2">
                  <span
                    aria-hidden="true"
                    class="iconify icon-[uil--key-skeleton] shrink-0 text-base text-muted"
                  ></span>
                  <span class="min-w-0 wrap-anywhere">{provider.label}</span>
                </span>
              </Pill>
            </li>
          {/each}
        </ul>
      {:else}
        <p class="mt-1 text-muted">{m('admin.registration.no_methods')}</p>
      {/if}
    </div>
  {/if}
</Panel>
