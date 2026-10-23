<!-- @component Shared room-group frame and data lifecycle for its routed sections. -->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { provideRoomGroupManagement } from './roomGroupManagementContext';
  import { errorMessage } from '$lib/utils/errorMessage';
  import { serverUi } from '$lib/state/server/serverUi';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminRoomLayoutAPI, type AdminManagedRoomGroup } from '$lib/api/adminRoomLayout';
  import { createSessionGuard, type SessionSnapshot } from '$lib/state/server/sessionGuard.svelte';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Button } from '$lib/ui/form';
  import {
    AccessDenied,
    EmptyState,
    PaneContent,
    PaneHeader,
    PageTitle,
    LoadingFog,
    TabNav,
    type TabNavItem
  } from '$lib/ui';
  import { useProjectionEvent } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { adminQueryKeys } from '$lib/query/admin';
  import { createMutation, createQuery, queryClient } from '$lib/query/client';
  import {
    invalidateAdminRoomLayoutQueries,
    purgeAdminRoomGroupQuery
  } from '$lib/query/adminInvalidation';
  import type { buildRoomGroupSettingsUpdate } from './roomGroupSettings';
  import { m } from '$lib/i18n/messages';

  let { children }: { children: Snippet } = $props();

  const serverScope = useServerScope();
  const groupId = $derived(page.params.groupId!);
  const activeServerId = serverScope.serverId;
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const backHref = $derived(resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }));

  const session = createSessionGuard(serverScope);
  /** Increases when the group snapshot changes, so an older save does not overwrite it. */
  let snapshotGeneration = 0;
  let formRevision = $state(0);

  type GroupMutationScope = SessionSnapshot & {
    groupId: string;
    queryKey: ReturnType<typeof adminQueryKeys.roomGroup>;
    api: ReturnType<typeof createAdminRoomLayoutAPI>;
    snapshotGeneration: number;
    input: ReturnType<typeof buildRoomGroupSettingsUpdate>;
  };

  const groupQuery = createQuery(() => {
    const serverId = activeServerId;
    const connection = serverScope.connection;
    const targetGroupId = groupId;
    return {
      queryKey: adminQueryKeys.roomGroup(serverId, connection, targetGroupId),
      queryFn: ({ signal }) =>
        connection.getAPI(createAdminRoomLayoutAPI).getRoomGroup(targetGroupId, { signal }),
      refetchOnMount: 'always' as const
    };
  });

  const groupDetails = $derived(groupQuery.data ?? null);
  const group = $derived(groupDetails?.group ?? null);
  const canManageGroup = $derived(groupDetails?.canManageGroup ?? false);
  const canManagePermissions = $derived(groupDetails?.canManagePermissions ?? false);
  const loading = $derived(groupQuery.isPending);
  const classifiedLoadError = $derived(
    groupQuery.error ? classifyManagementLoadError(groupQuery.error) : null
  );
  const accessDenied = $derived(
    classifiedLoadError?.kind === 'access-denied' || (!loading && !group)
  );
  const loadFailure = $derived(
    classifiedLoadError?.kind === 'failure' ? classifiedLoadError.message : null
  );

  function isCurrentGroup(variables: GroupMutationScope | undefined): boolean {
    return session.isCurrent(variables) && variables.groupId === groupId;
  }

  function canApplyGroupSnapshot(variables: GroupMutationScope): boolean {
    return isCurrentGroup(variables) && variables.snapshotGeneration === snapshotGeneration;
  }

  const updateGroupMutation = createMutation(() => ({
    mutationFn: async ({ api, input }: GroupMutationScope) => {
      const updated = await api.updateRoomGroup(input);
      if (!updated) throw new Error('Room group update returned no group');
      return updated;
    },
    onSuccess: (updated, variables) => {
      if (!isCurrentGroup(variables)) return;
      if (canApplyGroupSnapshot(variables)) {
        queryClient.setQueryData<AdminManagedRoomGroup | null>(variables.queryKey, (current) =>
          current ? { ...current, group: updated } : current
        );
        formRevision += 1;
      }
      invalidateAdminRoomLayoutQueries(
        variables.serverId,
        variables.connection,
        undefined,
        variables.groupId
      );
      void serverUi(serverScope.store).adminRoomLayout.refresh();
      toast.success(m('admin.rooms_admin.group_renamed'));
    },
    onError: (error, variables) => {
      if (!isCurrentGroup(variables)) return;
      toast.error(
        m('admin.rooms_admin.rename_group_failed', {
          error: errorMessage(error)
        })
      );
    }
  }));

  function saveGeneralSettings(input: ReturnType<typeof buildRoomGroupSettingsUpdate>): void {
    if (!canManageGroup || updateGroupMutation.isPending) return;
    const snapshot = session.snapshot();
    updateGroupMutation.mutate({
      ...snapshot,
      groupId,
      queryKey: adminQueryKeys.roomGroup(snapshot.serverId, snapshot.connection, groupId),
      api: snapshot.connection.getAPI(createAdminRoomLayoutAPI),
      snapshotGeneration,
      input
    });
  }

  useProjectionEvent((event) => {
    if (event.resource?.case === 'roomGroups') {
      snapshotGeneration += 1;
      if (event.resource.value.groups.some((candidate) => candidate.id === groupId)) {
        invalidateAdminRoomLayoutQueries(
          activeServerId,
          serverScope.connection,
          undefined,
          groupId
        );
      } else {
        session.invalidate();
        purgeAdminRoomGroupQuery(activeServerId, serverScope.connection, groupId);
      }
    }
  });

  const saving = $derived(
    updateGroupMutation.isPending && isCurrentGroup(updateGroupMutation.variables)
  );

  provideRoomGroupManagement({
    get groupId() {
      return groupId;
    },
    get group() {
      return group!;
    },
    get canManageGroup() {
      return canManageGroup;
    },
    get saving() {
      return saving;
    },
    get formRevision() {
      return formRevision;
    },
    saveGeneralSettings
  });
  const isPermissionsSection = $derived(
    page.route.id === '/chat/[serverId]/manage/room-groups/[groupId]/permissions'
  );
  const sectionTabs = $derived.by<TabNavItem[]>(() => {
    if (!group) return [];
    const params = { serverId: serverSegment, groupId };
    const items: TabNavItem[] = [];
    if (canManageGroup)
      items.push({
        href: resolve('/chat/[serverId]/manage/room-groups/[groupId]', params),
        label: m('admin.rooms_admin.tabs.general'),
        icon: 'icon-[uil--sliders-v-alt]',
        current: !isPermissionsSection
      });
    items.push({
      href: resolve('/chat/[serverId]/manage/room-groups/[groupId]/permissions', params),
      label: m('admin.rooms_admin.tabs.permissions'),
      icon: 'icon-[uil--shield-check]',
      current: isPermissionsSection
    });
    return items;
  });
  const pageTitle = $derived(
    group
      ? `${group.name} · ${m('admin.rooms_admin.rename_group')}`
      : m('admin.rooms_admin.rename_group')
  );
</script>

<PageTitle title={m('admin.common.server_admin_page_title', { title: pageTitle })} />

{#if !loading && loadFailure}
  <EmptyState icon="icon-[uil--exclamation-triangle]" title={m('common.error.generic')}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void groupQuery.refetch()}>
        {m('common.retry')}
      </Button>
    </div>
  </EmptyState>
{:else if !loading && (accessDenied || !group || !canManagePermissions)}
  <AccessDenied
    message={m('ui.access_denied.message')}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m('chat.management_nav.back_to_server')}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={group?.name ?? m('admin.rooms_admin.rename_group')}
      subtitle={m('admin.rooms_admin.rename_group')}
      {backHref}
      backLabel={m('admin.rooms_admin.back_to_rooms')}
    >
      {#snippet tabs()}
        <TabNav label={m('admin.rooms_admin.rename_group')} items={sectionTabs} />
      {/snippet}
    </PaneHeader>
    <PaneContent fillHeight={isPermissionsSection}>
      <div class={['flex flex-col gap-6', isPermissionsSection && 'min-h-0 flex-1']}>
        {#if group}
          {@render children()}
        {:else}
          <LoadingFog class="h-40 w-full" />
        {/if}
      </div>
    </PaneContent>
  </div>
{/if}
