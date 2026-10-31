<script lang="ts">
  import {
    createRegistrationSettingsAPI,
    type RegistrationAdmission
  } from '$lib/api/registrationSettings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, queryClient } from '$lib/query/client';
  import { errorMessage, toastError } from '$lib/utils/errorMessage';
  import { m } from '$lib/i18n/messages';
  import { Panel, Hint, LoadingFog, ChoiceRow } from '$lib/ui';
  import { toast } from '$lib/ui/toast';

  const scope = useServerScope();
  const session = createSessionGuard(scope);
  const key = adminQueryKeys.registrationSettings(scope.serverId, scope.connection);
  let {
    admission,
    loading,
    error
  }: {
    admission: RegistrationAdmission | undefined;
    loading: boolean;
    error: unknown;
  } = $props();
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
      // Refresh public admission hints used by member-facing navigation.
      void scope.store.serverInfo.refreshProfile();
      toast.success(m('admin.security.settings_saved'));
    },
    onError: (error, variables) => {
      if (session.isCurrent(variables.snapshot)) toastError(error);
    }
  }));

  function save(value: RegistrationAdmission) {
    if (value === admission || mutation.isPending) return;
    mutation.mutate({
      admission: value,
      snapshot: session.snapshot()
    });
  }
</script>

<!-- @component Controls admission for new accounts on this server. -->
<Panel title={m('admin.registration.title')}>
  {#if loading}
    <LoadingFog class="h-24 w-full" />
  {:else if error}
    <Hint tone="danger">{errorMessage(error)}</Hint>
  {:else}
    <div role="radiogroup" aria-label={m('admin.registration.title')} class="flex flex-col gap-2">
      <ChoiceRow
        label={m('admin.registration.open')}
        description={m('admin.registration.open_description')}
        selected={admission === 'open'}
        disabled={mutation.isPending}
        onclick={() => save('open')}
      />
      <ChoiceRow
        label={m('admin.registration.invite_only')}
        description={m('admin.registration.invite_only_description')}
        selected={admission === 'invite_only'}
        disabled={mutation.isPending}
        onclick={() => save('invite_only')}
      />
    </div>
  {/if}
</Panel>
