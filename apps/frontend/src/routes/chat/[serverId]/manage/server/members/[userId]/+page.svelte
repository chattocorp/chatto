<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { createAdminUserManagementAPI } from '$lib/api-client/adminUsers';
  import { UserPermissionsMatrix } from '$lib/components/rbac';
  import * as m from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Hint, PaneContent } from '$lib/ui';
  import { FormError } from '$lib/ui/form';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import { MemberDetailStore } from './MemberDetailStore.svelte';
  import MemberIdentitySettings from './MemberIdentitySettings.svelte';
  import MemberOverviewPanel from './MemberOverviewPanel.svelte';
  import MemberRoleAssignments from './MemberRoleAssignments.svelte';

  const serverScope = useServerScope();
  const activeServerId = $derived(serverScope.serverId);
  const store = $derived(serverScope.store);
  const userId = $derived(page.params.userId!);
  const currentUser = $derived(store.currentUser);
  const memberDetail = new MemberDetailStore(() =>
    serverScope.connection.getAPI(createAdminUserManagementAPI)
  );
  const isSelf = $derived(currentUser.user?.id === userId);
  const canViewMemberEmails = $derived(isSelf || store.permissions.canAdminViewUsers);
  const canAdminManageAccounts = $derived(store.permissions.canAdminManageAccounts);
  const backHref = $derived(
    resolve('/chat/[serverId]/manage/server/members', {
      serverId: serverIdToSegment(activeServerId)
    })
  );

  $effect(() => {
    void memberDetail.setMember(activeServerId, userId);
  });
</script>

<PageTitle
  title={m['admin.common.server_admin_page_title']({
    title: memberDetail.member?.displayName ?? m['admin.members.member_fallback']()
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m['admin.members.member_details']()}
    subtitle={memberDetail.member?.displayName ?? m['common.loading']()}
    {backHref}
    backLabel={m['admin.members.back_to_members']()}
    showMobileNav
  />

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if memberDetail.loading}
        <div class="text-muted">{m['admin.members.loading_member']()}</div>
      {:else if !memberDetail.member}
        <Hint tone="danger">{m['admin.members.not_found']()}</Hint>
      {:else}
        {#if memberDetail.error}
          <FormError error={memberDetail.error} />
        {/if}

        <MemberOverviewPanel
          member={memberDetail.member}
          roles={memberDetail.roles}
          {canViewMemberEmails}
        />

        {#if canAdminManageAccounts}
          <MemberIdentitySettings store={memberDetail} {isSelf} />
        {/if}

        <MemberRoleAssignments store={memberDetail} {isSelf} serverId={activeServerId} />

        {#if memberDetail.canManageUserPermissions}
          <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
          <UserPermissionsMatrix {userId} />
        {/if}
      {/if}
    </div>
  </PaneContent>
</div>
