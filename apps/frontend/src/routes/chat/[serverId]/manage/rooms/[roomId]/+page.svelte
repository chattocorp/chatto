<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { createRoomDirectoryAPI, type DirectoryRoomDetails } from '$lib/api-client/roomDirectory';
  import { createAdminRoomLayoutAPI, type AdminManagedRoom } from '$lib/api-client/adminRoomLayout';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { createMemberDirectoryAPI } from '$lib/api-client/memberDirectory';
  import { Code, ConnectError } from '@connectrpc/connect';
  import { getChromePermissions } from '$lib/state/server/chromePermissions.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { supportsRoomManagerMemberReads } from '$lib/state/server/compatibility';
  import { useProjectionEvent } from '$lib/hooks';
  import { Panel } from '$lib/components/admin';
  import { Button, Checkbox, TextArea, TextInput } from '$lib/ui/form';
  import AccessDenied from '$lib/ui/AccessDenied.svelte';
  import { EmptyState, PaneContent } from '$lib/ui';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';
  import Hint from '$lib/ui/Hint.svelte';
  import PermissionMatrix from '$lib/components/rbac/PermissionMatrix.svelte';
  import { toast } from '$lib/ui/toast';
  import { UNIVERSAL_ROOM_HELP_TEXT } from '$lib/utils/roomCopy';
  import { classifyManagementLoadError } from '$lib/utils/managementLoadError';
  import { buildRoomSettingsUpdate } from './roomSettings';
  import RoomMembersPanel from './RoomMembersPanel.svelte';
  import { RoomMemberManagementStore } from './RoomMemberManagementStore.svelte';
  import * as m from '$lib/i18n/messages';

  const roomId = $derived(page.params.roomId!);
  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
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
  let identityGeneration = 0;
  let scrollContainer = $state<HTMLDivElement>();

  const memberManagement = new RoomMemberManagementStore((serverId) => {
    const conn = serverConnectionManager.getClient(serverId);
    return {
      directory: createMemberDirectoryAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      }),
      commands: createRoomCommandAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      })
    };
  });

  const canManageRoom = $derived(room?.canManageRoom ?? false);
  const canManagePermissions = $derived(room?.canManagePermissions ?? false);
  const supportsMemberManagement = $derived.by(() => {
    const info = serverRegistry.tryGetStore(activeServerId)?.serverInfo;
    if (!info) return false;
    return supportsRoomManagerMemberReads(info.protocolCapabilities, info.version);
  });
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

  function isCurrentLoad(requestId: number, targetServerId: string, targetRoomId: string): boolean {
    return requestId === loadId && targetServerId === activeServerId && targetRoomId === roomId;
  }

  function isCurrentIdentity(target: {
    serverId: string;
    roomId: string;
    identityGeneration: number;
  }): boolean {
    return (
      target.serverId === activeServerId &&
      target.roomId === roomId &&
      target.identityGeneration === identityGeneration
    );
  }

  async function loadRoom(
    targetServerId: string,
    targetRoomId: string,
    preserveRoom = false
  ): Promise<void> {
    const thisId = ++loadId;
    if (!preserveRoom) {
      loading = true;
      room = null;
      saving = false;
    }
    accessDenied = false;
    loadFailure = null;
    try {
      const conn = serverConnectionManager.getClient(targetServerId);
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
      if (!isCurrentLoad(thisId, targetServerId, targetRoomId)) return;
      if (nextRoom) {
        applyRoom(nextRoom);
      } else {
        accessDenied = true;
      }
    } catch (error) {
      if (!isCurrentLoad(thisId, targetServerId, targetRoomId)) return;
      const classified = classifyManagementLoadError(error);
      if (classified.kind === 'access-denied') {
        accessDenied = true;
      } else {
        loadFailure = classified.message;
      }
    } finally {
      if (isCurrentLoad(thisId, targetServerId, targetRoomId)) loading = false;
    }
  }

  $effect(() => {
    const targetServerId = activeServerId;
    const targetRoomId = roomId;
    untrack(() => {
      identityGeneration++;
      saving = false;
      void loadRoom(targetServerId, targetRoomId);
    });
  });

  useProjectionEvent((event) => {
    for (const operation of event.operations) {
      switch (operation.operation.case) {
        case 'roomUpsert':
          if (operation.operation.value.room?.room?.id === roomId) {
            void loadRoom(activeServerId, roomId, true);
            return;
          }
          break;
        case 'roomRemove':
          if (operation.operation.value.roomId === roomId) {
            identityGeneration++;
            saving = false;
            void loadRoom(activeServerId, roomId);
            return;
          }
          break;
      }
    }
  });

  async function saveGeneralSettings(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!canManageRoom || saving || nameError || !name.trim() || !changed) return;

    const target = {
      serverId: activeServerId,
      roomId,
      identityGeneration,
      loadId
    };
    const update = buildRoomSettingsUpdate(
      target.roomId,
      { name, description, universal },
      {
        name: originalName,
        description: originalDescription,
        universal: originalUniversal
      }
    );
    saving = true;
    try {
      const conn = serverConnectionManager.getClient(target.serverId);
      const api = createRoomCommandAPI({
        serverId: conn.serverId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      const updated = await api.updateRoom(update);
      if (!isCurrentIdentity(target)) return;
      if (!updated) throw new Error('Room update returned no room');

      if (target.loadId === loadId && room) {
        applyRoom({
          ...room,
          name: updated.name,
          description: updated.description || null,
          isUniversal: updated.universal,
          archived: updated.archived
        });
      }
      void serverRegistry.getStore(activeServerId).rooms.refresh();
      toast.success(m['admin.rooms_admin.room_updated']());
    } catch (error) {
      if (!isCurrentIdentity(target)) return;
      toast.error(
        m['admin.rooms_admin.update_room_failed']({
          error: error instanceof Error ? error.message : String(error)
        })
      );
    } finally {
      if (isCurrentIdentity(target)) saving = false;
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
      <Button variant="secondary" onclick={() => void loadRoom(activeServerId, roomId)}>
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

    <PaneContent bind:scrollContainer>
      <div class="flex flex-col gap-6">
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

        {#if supportsMemberManagement}
          <RoomMembersPanel
            serverId={activeServerId}
            {roomId}
            roomName={room.name}
            isUniversal={room.isUniversal}
            archived={room.archived}
            canManageMembers={canManageRoom}
            scrollRoot={scrollContainer}
            store={memberManagement}
          />
        {/if}

        <div class="flex flex-col gap-4">
          <h2 class="text-lg font-semibold text-text-top">
            {m['admin.rooms_admin.room_permissions_title_fallback']()}
          </h2>
          <Hint>{m['admin.rooms_admin.room_permissions_hint']()}</Hint>
          <Hint>{m['admin.permissions.resolution_hint']()}</Hint>
          <PermissionMatrix {roomId} scrollContents={false} />
        </div>
      </div>
    </PaneContent>
  </div>
{/if}
