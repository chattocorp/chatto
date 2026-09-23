<!-- @component Public bot owner identity, outside the collapsible permission section. -->
<script lang="ts">
  import type { ViewerTimeSettings } from '$lib/utils/formatTime';
  import { createQuery } from '@tanstack/svelte-query';
  import { createUserAPI } from '$lib/api-client/users';
  import UserIdentity from '$lib/components/users/UserIdentity.svelte';
  import { m } from '$lib/i18n/messages';
  import { queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { mapOptionalUserSummary } from '$lib/api-client/userSummary';
  import { getLiveDisplayName, getLiveAvatarUrl } from '$lib/state/userProfiles.svelte';

  let {
    ownerId,
    onSendMessage,
    onOpenProfile,
    viewerSettings
  }: {
    ownerId: string;
    onSendMessage?: (userId: string) => void;
    onOpenProfile?: (userId: string) => void;
    viewerSettings?: ViewerTimeSettings | null;
  } = $props();
  const scope = useServerScope();
  const users = $derived(scope.store.projection.users);
  const query = createQuery(
    () => ({
      queryKey: [
        'server',
        scope.serverId,
        'session',
        scope.connection.queryScope,
        'bot-owner',
        ownerId
      ],
      queryFn: async () => {
        const [owner] = await scope.connection.getAPI(createUserAPI).batchGetUsers([ownerId]);
        return owner ?? null;
      },
      staleTime: 30_000,
      refetchInterval: 30_000,
      gcTime: 0
    }),
    () => queryClient
  );
  const owner = $derived(mapOptionalUserSummary(users.get(ownerId)?.user));
  const identity = $derived(
    owner && !owner.deleted
      ? {
          ...owner,
          displayName: getLiveDisplayName(owner.id, owner.displayName || owner.login),
          avatarUrl: getLiveAvatarUrl(owner.id, owner.avatarUrl)
        }
      : null
  );
</script>

<div class="mt-3 flex min-w-0 items-center gap-2" data-testid="bot-owner">
  <span class="shrink-0 text-muted">{m('chat.profile.owned_by')}</span>
  {#if identity}
    {#key ownerId}
      <UserIdentity
        user={identity}
        size="xs"
        openOnClick
        {onSendMessage}
        {onOpenProfile}
        {viewerSettings}
      />
    {/key}
  {:else}
    <span class="text-muted" aria-busy={query.isPending}>
      {query.isPending
        ? m('common.loading')
        : users.isDeleted(ownerId)
          ? m('common.deleted_user')
          : m('common.unknown_user')}
    </span>
  {/if}
</div>
