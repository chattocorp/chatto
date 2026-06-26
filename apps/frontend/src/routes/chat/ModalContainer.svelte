<script lang="ts">
  import { page } from '$app/state';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { graphqlClientManager } from '$lib/state/server/graphqlClient.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';
  import SignOutDialog from './SignOutDialog.svelte';

  const activeInstanceId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeInstanceId));
  import Dialog from '$lib/ui/Dialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import { Button } from '$lib/ui/form';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import { createRoomCommandAPI } from '$lib/api/rooms';
  import { createMessageAPI } from '$lib/api/messages';

  import ImageModal from '$lib/ui/ImageModal.svelte';

  import { refreshAttachmentUrlsForMessage } from '$lib/attachments/attachmentUrls';
  import { toast } from '$lib/ui/toast';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';

  function closeModal() {
    history.back();
  }

  /** Get the GraphQL client for read/refresh helpers that still use GraphQL. */
  function getActiveClient() {
    return graphqlClientManager.getClient(activeInstanceId).client;
  }

  function getActiveMessageAPI() {
    const conn = graphqlClientManager.getClient(activeInstanceId);
    return createMessageAPI({
      serverId: conn.serverId ?? activeInstanceId,
      baseUrl: conn.connectBaseUrl,
      bearerToken: conn.bearerToken
    });
  }

  function handleRoomCreated(roomId: string) {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  let leavingRoom = $state(false);
  let joiningRoom = $state(false);
  let leavingServer = $state(false);
  let deletingMessage = $state(false);
  let deletingLinkPreview = $state(false);
  let deletingAttachment = $state(false);

  // Keep the lightbox ahead of the one-hour access ticket expiry.
  const IMAGE_MODAL_URL_REFRESH_MS = 50 * 60 * 1000;

  async function handleLeaveRoom(roomId: string) {
    leavingRoom = true;
    try {
      const conn = graphqlClientManager.getClient(activeInstanceId);
      const api = createRoomCommandAPI({
        serverId: conn.serverId ?? activeInstanceId,
        baseUrl: conn.connectBaseUrl,
        bearerToken: conn.bearerToken
      });
      await api.leaveRoom(roomId);
    } catch (error) {
      leavingRoom = false;
      toast.error(m['room.leave.failed']());
      console.error('Error leaving room:', error);
      closeModal();
      return;
    }
    leavingRoom = false;

    clearLastRoom(activeInstanceId);
    goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
  }

  async function handleJoinRoom(roomId: string) {
    joiningRoom = true;
    const stores = serverRegistry.getStore(activeInstanceId);
    const result = await stores.roomDirectory.joinRoom(roomId);
    joiningRoom = false;

    if (!result.ok) {
      toast.error(m['room.join.failed']());
      console.error('Error joining room:', result.error);
      closeModal();
      return;
    }

    toast.success(
      result.room
        ? m['room.join.success']({ room: result.room.name })
        : m['room.join.success_generic']()
    );
    await stores.rooms.refresh();
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  async function handleLeaveServer() {
    // Post-#330 PR(a) "leave server" no longer hits the API — server membership
    // is implicit on signup, so the action is purely a client-side disconnect:
    // forget the instance from the registry and route somewhere safe.
    leavingServer = true;
    clearLastRoom(activeInstanceId);

    const leftInstanceId = activeInstanceId;
    serverRegistry.removeServer(leftInstanceId);

    // Land on the origin instance if it exists, otherwise root.
    const originId = serverRegistry.originServer?.id;
    if (originId && originId !== leftInstanceId) {
      goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(originId) }));
    } else {
      goto(resolve('/'));
    }
    leavingServer = false;
  }

  async function handleDeleteMessage(roomId: string, eventId: string) {
    deletingMessage = true;
    try {
      await getActiveMessageAPI().deleteMessage(roomId, eventId);
    } catch (error) {
      deletingMessage = false;
      toast.error(m['room.message.delete_failed']());
      console.error('Error deleting message:', error);
      closeModal();
      return;
    }
    deletingMessage = false;
    notifyRoomMessageMutated({ roomId, eventId, reason: 'message-deleted' });
    toast.success(m['room.message.deleted']());
    closeModal();
  }

  async function handleDeleteLinkPreview(roomId: string, eventId: string, previewUrl: string) {
    deletingLinkPreview = true;
    try {
      await getActiveMessageAPI().deleteLinkPreview(roomId, eventId, previewUrl);
    } catch (error) {
      deletingLinkPreview = false;
      toast.error(m['room.link_preview.delete_failed']());
      console.error('Error deleting link preview:', error);
      closeModal();
      return;
    }
    deletingLinkPreview = false;
    notifyRoomMessageMutated({ roomId, eventId, reason: 'link-preview-deleted' });
    closeModal();
  }

  async function handleDeleteAttachment(roomId: string, eventId: string, attachmentId: string) {
    deletingAttachment = true;
    try {
      await getActiveMessageAPI().deleteAttachment(roomId, eventId, attachmentId);
    } catch (error) {
      deletingAttachment = false;
      toast.error(m['room.attachment.delete_failed']());
      console.error('Error deleting attachment:', error);
      closeModal();
      return;
    }
    deletingAttachment = false;
    notifyRoomMessageMutated({ roomId, eventId, reason: 'attachment-deleted' });
    closeModal();
  }

  async function refreshImageViewerUrls() {
    const modal = page.state.modal;
    if (modal?.type !== 'imageViewer' || !roomId || !eventId || !modal.imageItems?.length) {
      return;
    }
    const refreshRoomId = roomId;
    const refreshEventId = eventId;
    const freshUrls = await refreshAttachmentUrlsForMessage(
      getActiveClient(),
      refreshRoomId,
      refreshEventId
    );
    if (freshUrls.size === 0) {
      return;
    }
    const currentModal = page.state.modal;
    if (
      currentModal?.type !== 'imageViewer' ||
      currentModal.roomId !== refreshRoomId ||
      currentModal.eventId !== refreshEventId ||
      !currentModal.imageItems?.length
    ) {
      return;
    }
    const imageItems = currentModal.imageItems.map((item) => ({
      ...item,
      src: item.id ? (freshUrls.get(item.id)?.assetUrl.url ?? item.src) : item.src
    }));
    replaceState('', {
      ...page.state,
      modal: {
        ...currentModal,
        imageItems
      }
    });
  }

  const modalType = $derived(page.state.modal?.type);
  const roomId = $derived(page.state.modal?.roomId);
  const roomName = $derived(page.state.modal?.roomName);
  const spaceName = $derived(page.state.modal?.spaceName);
  const eventId = $derived(page.state.modal?.eventId);
  const attachmentId = $derived(page.state.modal?.attachmentId);
  const _attachmentFilename = $derived(page.state.modal?.attachmentFilename);
  const previewUrl = $derived(page.state.modal?.previewUrl);
  const imageItems = $derived(page.state.modal?.imageItems ?? []);
  const imageIndex = $derived(page.state.modal?.imageIndex ?? 0);

  $effect(() => {
    if (modalType !== 'imageViewer') {
      return;
    }

    const interval = window.setInterval(() => {
      refreshImageViewerUrls().catch((error: unknown) => {
        console.warn('Failed to refresh image viewer URLs', error);
      });
    }, IMAGE_MODAL_URL_REFRESH_MS);

    return () => window.clearInterval(interval);
  });
</script>

{#if modalType === 'createRoom'}
  <Dialog visible title={m['room.create.title']()} size="md" onclose={closeModal}>
    <p class="mb-4 text-muted">{m['room.create.description']()}</p>
    <CreateRoom onroomcreated={(roomId) => handleRoomCreated(roomId)} />
  </Dialog>
{:else if modalType === 'logout'}
  <SignOutDialog onclose={closeModal} />
{:else if modalType === 'joinRoom' && roomId}
  {#if page.state.modal?.viewerCanJoinRoom}
    <ConfirmDialog
      title={m['room.join.title']()}
      tone="info"
      actionLabel={m['room.join.action']()}
      actionIcon="iconify uil--plus"
      loading={joiningRoom}
      onconfirm={() => handleJoinRoom(roomId)}
      onclose={closeModal}
    >
      {m['room.join.prompt']({ room: roomName ?? '' })}
    </ConfirmDialog>
  {:else}
    <Dialog visible title={m['room.join.access_title']()} size="sm" onclose={closeModal}>
      {#snippet footer()}
        <div class="flex justify-end">
          <Button variant="accent" onclick={closeModal}>
            <span class="iconify uil--check"></span>
            {m['common.got_it']()}
          </Button>
        </div>
      {/snippet}

      <p class="text-muted">{m['room.join.access_denied']()}</p>
    </Dialog>
  {/if}
{:else if modalType === 'leaveRoom' && roomId}
  <ConfirmDialog
    title={m['room.leave.title']()}
    actionLabel={m['room.leave.action']()}
    actionIcon="iconify uil--sign-out-alt"
    loading={leavingRoom}
    onconfirm={() => handleLeaveRoom(roomId)}
    onclose={closeModal}
  >
    {m['room.leave.prompt']({ room: roomName ?? '' })}
  </ConfirmDialog>
{:else if modalType === 'leaveServer'}
  <ConfirmDialog
    title={m['room.server.leave_title']()}
    actionLabel={m['room.server.leave_action']()}
    actionIcon="iconify uil--sign-out-alt"
    loading={leavingServer}
    onconfirm={() => handleLeaveServer()}
    onclose={closeModal}
  >
    {m['room.server.leave_prompt']({ server: spaceName ?? '' })}
  </ConfirmDialog>
{:else if modalType === 'deleteMessage' && roomId && eventId}
  <ConfirmDialog
    title={m['room.message.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={deletingMessage}
    onconfirm={() => handleDeleteMessage(roomId, eventId)}
    onclose={closeModal}
  >
    {m['room.message.delete_prompt']()}
  </ConfirmDialog>
{:else if modalType === 'deleteAttachment' && roomId && eventId && attachmentId}
  <ConfirmDialog
    title={m['room.attachment.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={deletingAttachment}
    onconfirm={() => handleDeleteAttachment(roomId, eventId, attachmentId)}
    onclose={closeModal}
  >
    {m['room.attachment.delete_prompt']()}
  </ConfirmDialog>
{:else if modalType === 'deleteLinkPreview' && roomId && eventId && previewUrl}
  <ConfirmDialog
    title={m['room.link_preview.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={deletingLinkPreview}
    onconfirm={() => handleDeleteLinkPreview(roomId, eventId, previewUrl)}
    onclose={closeModal}
  >
    {m['room.link_preview.delete_prompt']()}
  </ConfirmDialog>
{:else if modalType === 'imageViewer' && imageItems.length > 0}
  <ImageModal items={imageItems} index={imageIndex} onclose={closeModal} />
{/if}
