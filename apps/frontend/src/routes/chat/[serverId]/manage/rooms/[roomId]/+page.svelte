<script lang="ts">
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { createRoomDirectoryAPI, type DirectoryRoomDetails } from '$lib/api-client/roomDirectory';
  import { createAdminRoomLayoutAPI, type AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { getChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { Panel } from '$lib/components/admin';
  import { Button, Checkbox, TextArea, TextInput } from '$lib/ui/form';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { EmptyState } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { isCurrentResourceOperation } from '$lib/utils/resourceOperationFence';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { buildRoomSettingsUpdate } from './roomSettings';
  import * as m from '$lib/i18n/messages';

  const roomId = $derived(page.params.roomId!);
  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const connection = useConnection();
  const chromePermissions = getChromePermissions();

  let room = $state<AdminManagedRoom | null>(null);
  let loading = $state(true);
  let accessDenied = $state(false);
  let loadFailure = $state<string | null>(null);
  let saving = $state(false);
  let name = $state('');
  let description = $state('');
  let universal = $state(false);
  let originalName = $state('');
  let originalDescription = $state('');
  let originalUniversal = $state(false);
  let loadId = 0;

  const canManageRoom = $derived(room?.canManageRoom ?? false);
  const canManagePermissions = $derived(room?.canManagePermissions ?? false);
  const backHref = $derived(
    chromePermissions.current.canManageRooms
      ? resolve('/chat/[serverId]/manage/rooms', { serverId: serverSegment })
      : resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId })
  );
  const nameError = $derived.by(() => {
    if (!name) return undefined;
    if (name.trim() === '') return m['admin.rooms_admin.room_name_empty']();
    if (name !== name.trim()) return m['admin.rooms_admin.room_name_trim']();
    if (!/^[a-zA-Z0-9_-]+$/.test(name.trim())) {
      return m['admin.rooms_admin.room_name_charset']();
    }
    if (name.length > 30) return m['admin.rooms_admin.room_name_too_long']();
    return undefined;
  });
  const changed = $derived(
    name.trim() !== originalName ||
      description.trim() !== originalDescription ||
      universal !== originalUniversal
  );

  function applyRoom(nextRoom: AdminManagedRoom): void {
    room = nextRoom;
    name = nextRoom.name;
    description = nextRoom.description ?? '';
    universal = nextRoom.isUniversal;
    originalName = nextRoom.name;
    originalDescription = nextRoom.description ?? '';
    originalUniversal = nextRoom.isUniversal;
  }

  async function loadRoom(targetRoomId: string): Promise<void> {
    const thisId = ++loadId;
    loading = true;
    saving = false;
    room = null;
    accessDenied = false;
    loadFailure = null;
    try {
      const conn = connection();
      const adminAPI = createAdminRoomLayoutAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      let nextRoom: AdminManagedRoom | null;
      try {
        nextRoom = await adminAPI.getRoom(targetRoomId);
      } catch (error) {
        if (ConnectError.from(error).code !== Code.Unimplemented) throw error;
        const directoryAPI = createRoomDirectoryAPI({
          serverId: conn.serverId,
          baseUrl: conn.connectBaseUrl,
          bearerToken: conn.bearerToken
        });
        const legacyRoom: DirectoryRoomDetails | null = await directoryAPI.getRoom(targetRoomId);
        nextRoom = legacyRoom
          ? {
              id: legacyRoom.id,
              name: legacyRoom.name,
              description: legacyRoom.description,
              archived: legacyRoom.archived,
              isUniversal: legacyRoom.isUniversal,
              canManageRoom: legacyRoom.canManageRoom,
              canManagePermissions:
                legacyRoom.canManageRoom || chromePermissions.current.canManageRoles
            }
          : null;
      }
      if (thisId !== loadId) return;
      if (nextRoom) {
        applyRoom(nextRoom);
      } else {
        accessDenied = true;
      }
    } catch (error) {
      if (thisId !== loadId) return;
      const classified = classifyManagementLoadError(error);
      if (classified.kind === 'access-denied') {
        accessDenied = true;
      } else {
        loadFailure = classified.message;
      }
    } finally {
      if (thisId === loadId) loading = false;
    }
  }

  $effect(() => {
    void loadRoom(roomId);
  });

  async function saveGeneralSettings(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canManageRoom || saving || nameError || !name.trim() || !changed) return;

    const target = { resourceId: roomId, generation: loadId };
    const update = buildRoomSettingsUpdate(
      target.resourceId,
      { name, description, universal },
      {
        name: originalName,
        description: originalDescription,
        universal: originalUniversal
      }
    );
    saving = true;
    try {
      const conn = connection();
      const api = createRoomCommandAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const updated = await api.updateRoom(update);
      if (!isCurrentResourceOperation(target, roomId, loadId)) return;
      if (!updated || !room) throw new Error('Room update returned no room');

      applyRoom({
        ...room,
        name: updated.name,
        description: updated.description || null,
        isUniversal: updated.universal,
        archived: updated.archived
      });
      void serverRegistry.getStore(activeServerId).rooms.refresh();
      toast.success(m['admin.rooms_admin.room_updated']());
    } catch (error) {
      if (!isCurrentResourceOperation(target, roomId, loadId)) return;
      toast.error(
        m['admin.rooms_admin.update_room_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    } finally {
      if (isCurrentResourceOperation(target, roomId, loadId)) saving = false;
    }
  }

  const pageTitle = $derived(
    room ? `#${room.name} · ${m['room_list.room_settings']()}` : m['room_list.room_settings']()
  );
</script>

<PageTitle title={m['admin.common.server_admin_page_title']({ title: pageTitle })} />

{#if loading}
  <!-- The management shell remains visible while the room capability loads. -->
{:else if loadFailure}
  <EmptyState icon="uil--exclamation-triangle" title={m['common.error.generic']()}>
    <div class="flex flex-col items-center gap-4">
      <p>{loadFailure}</p>
      <Button variant="secondary" onclick={() => void loadRoom(roomId)}>
        {m['common.retry']()}
      </Button>
    </div>
  </EmptyState>
{:else if accessDenied || !room || !canManagePermissions}
  <AccessDenied
    message={m['ui.access_denied.message']()}
    backHref={resolve('/chat/[serverId]', { serverId: serverSegment })}
    backLabel={m['admin.nav.back_to_server']()}
  />
{:else}
  <div class="pane-page">
    <PaneHeader
      title={`#${room.name}`}
      subtitle={m['room_list.room_settings']()}
      {backHref}
      showMobileNav
    />

    <div class="flex flex-col gap-6 overflow-y-auto p-6">
      {#if canManageRoom}
        <Panel title={m['admin.nav.general']()} icon="iconify uil--setting">
          <form class="flex max-w-2xl flex-col gap-4" onsubmit={saveGeneralSettings}>
            <TextInput
              id="room-settings-name"
              label={m['rbac.role_form.name']()}
              bind:value={name}
              required
              disabled={saving}
              error={nameError}
            />
            <TextArea
              id="room-settings-description"
              label={m['rbac.role_form.description']()}
              bind:value={description}
              rows={3}
              disabled={saving}
              placeholder={m['admin.rooms_admin.room_description_placeholder']()}
            />
            <Checkbox
              id="room-settings-universal"
              bind:checked={universal}
              disabled={saving}
              label={m['admin.rooms_admin.universal_room']()}
              description={UNIVERSAL_ROOM_HELP_TEXT}
            />
            <div class="flex justify-end">
              <Button
                type="submit"
                loading={saving}
                disabled={!name.trim() || !!nameError || !changed}
              >
                {m['admin.permissions.save_changes']()}
              </Button>
            </div>
          </form>
        </Panel>
      {/if}

      <div class="flex flex-col gap-4">
        <h2 class="text-lg font-semibold text-text-top">
          {m['admin.rooms_admin.room_permissions_title_fallback']()}
        </h2>
        <Hint>{m['admin.rooms_admin.room_permissions_hint']()}</Hint>
        <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
        <PermissionMatrix {roomId} />
      </div>
    </div>
  </div>
{/if}
