<!--
@component

Roles section of a bot: its server role assignments. Bots can hold every role
except owner. The member details of the bot tell which roles the viewer may
assign or revoke.
-->
<script lang="ts">
  import { createAdminUserManagementAPI, type AdminMemberDetails } from '$lib/api/adminUsers';
  import MemberRoleAssignments from '$lib/components/rbac/MemberRoleAssignments.svelte';
  import { m } from '$lib/i18n/messages';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createQuery, queryClient } from '$lib/query/client';
  import { settingsQueryKeys } from '$lib/query/settings';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import { Hint, LoadingFog } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { useBotDetail } from '../botDetailContext';

  const serverScope = useServerScope();
  const detail = useBotDetail();
  const session = createSessionGuard(serverScope);

  const memberQuery = createQuery(() => {
    const serverId = serverScope.serverId;
    const connection = serverScope.connection;
    const botId = detail.botId;
    return {
      queryKey: adminQueryKeys.member(serverId, connection, botId),
      queryFn: ({ signal }) =>
        connection.getAPI(createAdminUserManagementAPI).getMember(botId, { signal })
    };
  });

  // Tag mutation state with the bot: SvelteKit reuses this page for another bot.
  let updating = $state.raw<{ botId: string; roleName: string } | null>(null);
  let failure = $state.raw<{ botId: string; message: string } | null>(null);
  const updatingRole = $derived(updating?.botId === detail.botId ? updating.roleName : null);
  const visibleFailure = $derived(failure?.botId === detail.botId ? failure.message : null);

  async function toggleBotRole(roleName: string, currentlyHasRole: boolean): Promise<boolean> {
    if (updatingRole) return false;
    const snapshot = session.snapshot();
    const botId = detail.botId;
    const isCurrent = () => session.isCurrent(snapshot) && detail.isCurrentTarget(botId);
    const api = snapshot.connection.getAPI(createAdminUserManagementAPI);
    const pending = { botId, roleName };
    updating = pending;
    failure = null;
    try {
      const result = currentlyHasRole
        ? await api.revokeRole(botId, roleName)
        : await api.assignRole(botId, roleName);
      if (!isCurrent() || !result.changed) return false;
      const memberKey = adminQueryKeys.member(snapshot.serverId, snapshot.connection, botId);
      if (result.member) {
        const member = result.member;
        queryClient.setQueryData<AdminMemberDetails>(memberKey, (current) =>
          current ? { ...current, member } : current
        );
      }
      // A new rank changes which roles the viewer may change and whether the
      // viewer still outranks the bot.
      void queryClient.invalidateQueries({ queryKey: memberKey, exact: true });
      void queryClient.invalidateQueries({
        queryKey: settingsQueryKeys.bot(snapshot.serverId, snapshot.connection, botId),
        exact: true
      });
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.membersRoot(snapshot.serverId, snapshot.connection)
      });
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.userPermissions(snapshot.serverId, snapshot.connection, botId),
        exact: true
      });
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.roleMembers(snapshot.serverId, snapshot.connection, roleName),
        exact: true
      });
      return true;
    } catch (error) {
      if (isCurrent()) {
        failure = { botId, message: errorMessage(error, m('admin.members.role_update_failed')) };
      }
      return false;
    } finally {
      if (updating === pending) updating = null;
    }
  }
</script>

{#if memberQuery.isPending}
  <LoadingFog class="h-40 w-full" />
{:else if memberQuery.error}
  <Hint tone="danger">{errorMessage(memberQuery.error)}</Hint>
{:else if memberQuery.data}
  {#if visibleFailure}
    <FormError error={visibleFailure} />
  {/if}
  {#key detail.botId}
    <MemberRoleAssignments
      details={memberQuery.data}
      isSelf={false}
      serverId={serverScope.serverId}
      {updatingRole}
      toggleMemberRole={toggleBotRole}
    />
  {/key}
{/if}
