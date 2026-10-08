<!--
@component

Tells who can find and join a channel room, or the rooms of a room group. New
rooms and room groups start closed (ADR-116), so a closed room shows a warning.
The server computes the summary with the same resolver as authorization. Pass
exactly one of `roomId` and `groupId`.
-->
<script lang="ts">
  import { Hint } from '$lib/ui';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createPermissionAPI, type AccessSummary } from '@chatto/client/api/permissions';
  import { m } from '$lib/i18n/messages';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery } from '$lib/query/client';

  let {
    roomId = null,
    groupId = null
  }: {
    roomId?: string | null;
    groupId?: string | null;
  } = $props();

  const serverScope = useServerScope();

  const summaryQuery = createQuery(() => {
    const activeRoomId = roomId ?? null;
    const activeGroupId = groupId ?? null;
    return {
      queryKey: adminQueryKeys.accessSummary(
        serverScope.serverId,
        serverScope.connection,
        activeRoomId,
        activeGroupId
      ),
      queryFn: ({ signal }) =>
        serverScope.connection
          .getAPI(createPermissionAPI)
          .getAccessSummary({ roomId: activeRoomId, groupId: activeGroupId }, { signal })
    };
  });

  const message = $derived(describe(summaryQuery.data, groupId != null));

  function describe(
    summary: AccessSummary | undefined,
    group: boolean
  ): { tone: 'info' | 'warning'; text: string } | null {
    if (!summary) return null;
    if (summary.everyoneCanJoin && !summary.everyoneCanRead) {
      return {
        tone: 'warning',
        text: group ? m('rbac.access.group_join_no_read') : m('rbac.access.room_join_no_read')
      };
    }
    if (summary.everyoneCanJoin) {
      return {
        tone: 'info',
        text: summary.everyoneCanList
          ? group
            ? m('rbac.access.group_open')
            : m('rbac.access.room_open')
          : group
            ? m('rbac.access.group_open_unlisted')
            : m('rbac.access.room_open_unlisted')
      };
    }
    if (summary.rolesCanJoin.length > 0) {
      const roles = summary.rolesCanJoin.join(', ');
      return {
        tone: 'info',
        text: group
          ? m('rbac.access.group_roles', { roles })
          : m('rbac.access.room_roles', { roles })
      };
    }
    return {
      tone: 'warning',
      text: group ? m('rbac.access.group_closed') : m('rbac.access.room_closed')
    };
  }
</script>

{#if message}
  <div data-testid="access-summary">
    <Hint
      tone={message.tone}
      icon={message.tone === 'warning' ? 'icon-[uil--lock]' : 'icon-[uil--users-alt]'}
    >
      {message.text}
    </Hint>
  </div>
{/if}
