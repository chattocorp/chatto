<!--
@component

Shared frame of the member detail sections. It loads the member, renders the
header and the section tabs, and provides the member to the Profile, Account,
Roles, and Permissions pages through `memberDetailContext`. The account
deletion page is outside this route group and has its own frame.
-->
<script lang="ts">
  import AccountName from '$lib/components/users/AccountName.svelte';
  import { formatAccountName } from '@chatto/client/timeline/accountName';
  import { onDestroy, type Snippet } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import {
    createAdminUserManagementAPI,
    type AdminMember,
    type AdminMemberDetails
  } from '$lib/api/adminUsers';
  import { m } from '$lib/i18n/messages';
  import { serverIdToSegment } from '$lib/navigation';
  import { adminQueryKeys } from '$lib/query/admin';
  import { registerAdminUserRemovalListener } from '$lib/query/cacheRegistry';
  import { createQuery, queryClient } from '$lib/query/client';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { createSessionGuard } from '$lib/state/server/sessionGuard.svelte';
  import {
    Hint,
    PaneContent,
    LoadingFog,
    PaneHeader,
    PageTitle,
    TabNav,
    type TabNavItem
  } from '$lib/ui';
  import { provideMemberDetail, type MemberMutationScope } from '../memberDetailContext';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();
  const activeServerId = serverScope.serverId;
  const serverSegment = serverIdToSegment(activeServerId);
  const store = serverScope.store;
  const userId = $derived(page.params.userId!);
  const isSelf = $derived(store.viewerId === userId);
  const canViewMemberEmails = $derived(isSelf || store.permissions.canAdminViewUsers);
  const canAdminManageAccounts = $derived(store.permissions.canAdminManageAccounts);
  const backHref = resolve('/chat/[serverId]/manage/server/members', { serverId: serverSegment });

  const session = createSessionGuard(serverScope);
  let removedMember = $state<{ serverId: string; userId: string } | null>(null);

  const removeUserRemovalListener = registerAdminUserRemovalListener((serverId, removedUserId) => {
    if (serverId !== activeServerId || removedUserId !== userId) return;
    session.invalidate();
    removedMember = { serverId, userId: removedUserId };
  });

  onDestroy(removeUserRemovalListener);

  const memberQuery = createQuery(() => {
    const serverId = activeServerId;
    const connection = serverScope.connection;
    const targetUserId = userId;
    const removed = removedMember?.serverId === serverId && removedMember.userId === targetUserId;
    return {
      queryKey: adminQueryKeys.member(serverId, connection, targetUserId),
      queryFn: ({ signal }) =>
        connection.getAPI(createAdminUserManagementAPI).getMember(targetUserId, { signal }),
      enabled: !!serverId && !!targetUserId && !removed
    };
  });

  const details = $derived(memberQuery.data ?? null);
  const member = $derived(details?.member ?? null);
  const isBot = $derived(member?.isBot === true);
  // Self-deletion stays in the account settings danger zone; bots cascade with
  // their owner and cannot be deleted directly.
  const canDeleteHere = $derived(
    !isSelf && !isBot && !!member && !member.deleted && member.viewerCanDeleteAccount
  );
  const loading = $derived(memberQuery.isPending && memberQuery.isEnabled);

  function isCurrentTarget(target: MemberMutationScope | undefined): target is MemberMutationScope {
    return session.isCurrent(target) && target.userId === userId;
  }

  provideMemberDetail({
    get userId() {
      return userId;
    },
    get details() {
      return details!;
    },
    get member() {
      return member!;
    },
    get isSelf() {
      return isSelf;
    },
    get isBot() {
      return isBot;
    },
    get canAdminManageAccounts() {
      return canAdminManageAccounts;
    },
    get canViewMemberEmails() {
      return canViewMemberEmails;
    },
    get canDeleteHere() {
      return canDeleteHere;
    },
    get loadError() {
      return memberQuery.error;
    },
    mutationScope() {
      if (!member) return null;
      const snapshot = session.snapshot();
      return {
        ...snapshot,
        userId,
        queryKey: adminQueryKeys.member(snapshot.serverId, snapshot.connection, userId),
        api: snapshot.connection.getAPI(createAdminUserManagementAPI)
      };
    },
    isCurrentTarget,
    updateCachedMember(target, update: (current: AdminMember) => AdminMember) {
      queryClient.setQueryData<AdminMemberDetails>(target.queryKey, (current) => {
        if (!current?.member) return current;
        return { ...current, member: update(current.member) };
      });
    },
    invalidateMemberLists(target) {
      const filters = {
        queryKey: adminQueryKeys.membersRoot(target.serverId, target.connection)
      };
      void queryClient.cancelQueries(filters).then(() => queryClient.invalidateQueries(filters));
    },
    invalidateRole(target, roleName) {
      void queryClient.invalidateQueries({
        queryKey: adminQueryKeys.roleMembers(target.serverId, target.connection, roleName),
        exact: true
      });
    }
  });

  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    if (!details || !member) return [];
    const params = { serverId: serverSegment, userId };
    const routeId = page.route.id;
    const base = '/chat/[serverId]/manage/server/members/[userId]/(sections)' as const;
    const items: TabNavItem[] = [
      {
        href: resolve(base, params),
        label: m('admin.members.tabs.profile'),
        icon: 'icon-[uil--user]',
        current: routeId === base
      }
    ];
    if (!isBot && (canAdminManageAccounts || canDeleteHere)) {
      items.push({
        href: resolve(`${base}/account`, params),
        label: m('admin.members.tabs.account'),
        icon: 'icon-[uil--key-skeleton]',
        current: routeId === `${base}/account`
      });
    }
    if (!isBot) {
      items.push({
        href: resolve(`${base}/roles`, params),
        label: m('admin.members.tabs.roles'),
        icon: 'icon-[uil--award]',
        current: routeId === `${base}/roles`
      });
    }
    if (!isBot && details.viewerCanManageUserPermissions) {
      items.push({
        href: resolve(`${base}/permissions`, params),
        label: m('admin.members.tabs.permissions'),
        icon: 'icon-[uil--shield-check]',
        current: routeId === `${base}/permissions`
      });
    }
    return items;
  });
</script>

{#snippet memberName()}
  {#if member}
    <AccountName name={member.displayName} identity={member} />
  {:else}
    <LoadingFog class="h-4 w-28" label={m('admin.members.loading_member')} />
  {/if}
{/snippet}

<PageTitle
  title={m('admin.common.server_admin_page_title', {
    title: formatAccountName(member?.displayName ?? m('admin.members.member_fallback'), member)
  })}
/>

<div class="pane-page">
  <PaneHeader
    title={m('admin.members.member_details')}
    subtitle={formatAccountName(member?.displayName ?? m('common.loading'), member)}
    subtitleContent={memberName}
    {backHref}
    backLabel={m('admin.members.back_to_members')}
  >
    {#snippet tabs()}
      <TabNav label={m('admin.members.tabs.label')} items={sectionTabs} />
    {/snippet}
  </PaneHeader>

  <PaneContent>
    <div class="flex flex-col gap-6">
      {#if loading}
        <LoadingFog class="h-40 w-full" label={m('admin.members.loading_member')} />
      {:else if !details || !member}
        <Hint tone="danger">{m('admin.members.not_found')}</Hint>
      {:else}
        {@render children()}
      {/if}
    </div>
  </PaneContent>
</div>
