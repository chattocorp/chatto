<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { createAdminRoomLayoutAPI, type AdminRoomGroup } from '$lib/api-client/adminRoomLayout';
  import { useServerScope } from '$lib/state/server/scope.svelte';
  import { Panel } from '$lib/components/admin';
  import { Button, TextArea, TextInput } from '$lib/ui/form';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { EmptyState } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import { isCurrentResourceOperation } from '$lib/utils/resourceOperationFence';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { buildRoomGroupSettingsUpdate } from './roomGroupSettings';
  import * as m from '$lib/i18n/messages';

  const serverScope = useServerScope();
  const groupId = $derived(page.params.groupId!);
  const activeServerId = $derived(serverScope.serverId);
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const backHref = $derived(resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment }));

  let group = $state<AdminRoomGroup | null>(null);
  let loading = $state(true);
  let accessDenied = $state(false);
  let loadFailure = $state<string | null>(null);
  let saving = $state(false);
  let name = $state('');
  let description = $state('');
  let originalName = $state('');
  let originalDescription = $state('');
  let canManageGroup = $state(false);
  let canManagePermissions = $state(false);
  let loadId = 0;
  const changed = $derived(
    name.trim() !== originalName || description.trim() !== originalDescription
  );

  function applyGroup(nextGroup: AdminRoomGroup): void {
    group = nextGroup;
    name = nextGroup.name;
    description = nextGroup.description ?? '';
    originalName = nextGroup.name;
    originalDescription = nextGroup.description ?? '';
  }

  async function loadGroup(targetServerId: string, targetGroupId: string) {
    if (targetServerId !== serverScope.serverId) return;
    const targetStore = serverScope.store;
    const targetConnection = serverScope.connection;
    const thisId = ++loadId;
    loading = true;
    saving = false;
    group = null;
    accessDenied = false;
    loadFailure = null;
    canManageGroup = false;
    canManagePermissions = false;
    try {
      const info = targetStore.serverInfo;
      if (!info?.supportsFeature('adminApi')) {
        accessDenied = true;
        return;
      }
      const details = await targetConnection
        .getAPI(createAdminRoomLayoutAPI)
        .getRoomGroup(targetGroupId);
      if (!serverScope.isCurrent() || thisId !== loadId || targetServerId !== activeServerId)
        return;
      if (details) {
        canManageGroup = details.canManageGroup;
        canManagePermissions = details.canManagePermissions;
        applyGroup(details.group);
      } else {
        accessDenied = true;
      }
    } catch (error) {
      if (!serverScope.isCurrent() || thisId !== loadId || targetServerId !== activeServerId)
        return;
      const classified = classifyManagementLoadError(error);
      if (classified.kind === 'access-denied') {
        accessDenied = true;
      } else {
        loadFailure = classified.message;
      }
    } finally {
      if (serverScope.isCurrent() && thisId === loadId && targetServerId === activeServerId) {
        loading = false;
      }
    }
  }

  $effect(() => {
    void loadGroup(activeServerId, groupId);
  });

  async function saveGeneralSettings(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canManageGroup || saving || !name.trim() || !changed) return;

    const target = { resourceId: groupId, generation: loadId };
    const targetConnection = serverScope.connection;
    const targetRoomLayout = serverScope.store.adminRoomLayout;
    const update = buildRoomGroupSettingsUpdate(
      target.resourceId,
      { name, description },
      { name: originalName, description: originalDescription }
    );
    saving = true;
    try {
      const api = targetConnection.getAPI(createAdminRoomLayoutAPI);
      const updated = await api.updateRoomGroup(update);
      if (!serverScope.isCurrent() || !isCurrentResourceOperation(target, groupId, loadId)) return;
      if (!updated) throw new Error('Room group update returned no group');

      applyGroup(updated);
      void targetRoomLayout.refresh();
      toast.success(m['admin.rooms_admin.group_renamed']());
    } catch (error) {
      if (!serverScope.isCurrent() || !isCurrentResourceOperation(target, groupId, loadId)) return;
      toast.error(
        m['admin.rooms_admin.rename_group_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    } finally {
      if (serverScope.isCurrent() && isCurrentResourceOperation(target, groupId, loadId)) {
        saving = false;
      }
    }
  }

  const pageTitle = $derived(
    group
      ? `${group.name} · ${m['admin.rooms_admin.rename_group']()}`
      : m['admin.rooms_admin.rename_group']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

{#if loading}
  <!-- The management shell remains visible while the room group loads. -->
{:else if loadFailure}
  <EmptyState icon="uil--exclamation-triangle" title={m['common.error.generic']()}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void loadGroup(activeServerId, groupId)}>
        {m['common.retry']()}
      </Button>
    </div>
  </EmptyState>
{:else if accessDenied || !group || !canManagePermissions}
  <AccessDenied
    message={m['ui.access_denied.message']()}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m['admin.nav.back_to_server']()}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={group.name}
      subtitle={m['admin.rooms_admin.rename_group']()}
      {backHref}
      backLabel={m['admin.rooms_admin.back_to_rooms']()}
      showMobileNav
    />

    <div class="flex flex-col gap-6 overflow-y-auto p-6">
      {#if canManageGroup}
        <Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
          <form class="flex max-w-2xl flex-col gap-4" onsubmit={saveGeneralSettings}>
            <TextInput
              id="room-group-settings-name"
              label={m['admin.rooms_admin.group_name']()}
              bind:value={name}
              required
              maxlength={80}
              disabled={saving}
            />
            <TextArea
              id="room-group-settings-description"
              label={m['rbac.role_form.description']()}
              bind:value={description}
              rows={3}
              maxlength={500}
              disabled={saving}
            />
            <div class="flex justify-end">
              <Button type="submit" loading={saving} disabled={!name.trim() || !changed}>
                {m['admin.permissions.save_changes']()}
              </Button>
            </div>
          </form>
        </Panel>
      {/if}

      <div class="flex flex-col gap-4">
        <h2 class="text-lg font-semibold text-text-top">
          {m['admin.rooms_admin.group_permissions_title_fallback']()}
        </h2>
        <Hint>{m['admin.rooms_admin.group_permissions_hint']()}</Hint>
        <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
        <PermissionMatrix {groupId} />
      </div>
    </div>
  </div>
{/if}
