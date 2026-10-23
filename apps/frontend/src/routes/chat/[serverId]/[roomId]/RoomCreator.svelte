<!-- @component Resolves the room creator through the shared user directory, including non-members. -->
<script lang="ts">
  import { createUserAPI } from '@chatto/client/api/users';
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { m } from '$lib/i18n/messages';
  import { createQuery } from '$lib/query/client';
  import { serverSessionQueryRoot } from '$lib/query/keys';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { LoadingFog } from '$lib/ui';

  let { userId }: { userId: string } = $props();
  const scope = useServerScope();
  const users = $derived(scope.store.projection.users);
  const query = createQuery(() => ({
    queryKey: [...serverSessionQueryRoot(scope.serverId, scope.connection), 'room-creator', userId],
    queryFn: async () => {
      const [user] = await scope.connection.getAPI(createUserAPI).batchGetUsers([userId]);
      return user ?? null;
    },
    staleTime: 30_000,
    refetchInterval: 30_000,
    gcTime: 0
  }));
  const creator = $derived(users.view(userId));
</script>

{#if creator && !creator.deleted}
  <AccountName name={creator.displayName || creator.login} identity={creator} />
{:else if query.isPending}
  <LoadingFog class="inline-block h-4 w-20" />
{:else}
  {users.isDeleted(userId) ? m('common.deleted_user') : m('common.unknown_user')}
{/if}
