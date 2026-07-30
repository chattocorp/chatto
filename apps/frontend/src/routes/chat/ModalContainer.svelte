<script lang="ts">
  import { version } from '$app/environment';
  import { page } from '$app/state';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { serverConnectionManager } from '$lib/state/server/serverConnection.svelte';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import * as m from '$lib/i18n/messages';
  import SignOutDialog from './SignOutDialog.svelte';

  const activeInstanceId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeInstanceId));
  const modal = $derived(page.state.modal);
  const modalServerId = $derived(modal?.serverId ?? activeInstanceId);
  import Dialog from '$lib/ui/Dialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';
  import { createRoomCommandAPI } from '$lib/api-client/rooms';
  import { createMessageAPI } from '$lib/api-client/messages';
  import { createAttachmentAPI } from '$lib/api-client/attachments';

  import ImageModal from '$lib/ui/ImageModal.svelte';

  import {
    LIGHTBOX_ATTACHMENT_IMAGE_REFRESH,
    refreshAttachmentUrlsForAssets
  } from '$lib/attachments/attachmentUrls';
  import { assetUrlForServer } from '$lib/assets/assetUrls';
  import { toast } from '$lib/ui/toast';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { notifyRoomMessageMutated } from '$lib/state/room/messageMutationEvents';

  let simulatedChattoWordmarkModule: Promise<
    typeof import('$lib/components/SimulatedChattoWordmark.svelte')
  > | null = null;

  function loadSimulatedChattoWordmark() {
    simulatedChattoWordmarkModule ??= import('$lib/components/SimulatedChattoWordmark.svelte');
    return simulatedChattoWordmarkModule;
  }

  function closeModal() {
    history.back();
  }

  function getActiveMessageAPI() {
    return serverConnectionManager.getClient(activeInstanceId).getAPI(createMessageAPI);
  }

  function getActiveAttachmentAPI() {
    return serverConnectionManager.getClient(activeInstanceId).getAPI(createAttachmentAPI);
  }

  function handleRoomCreated(roomId: string) {
    goto(resolve('/chat/[serverId]/[roomId]', { serverId: serverSegment, roomId }));
  }

  let pendingAction = $state<NonNullable<App.PageState['modal']>['type']>();

  // Preserve roughly an hour of margin ahead of the 23-hour minimum ticket validity.
  const IMAGE_MODAL_URL_REFRESH_MS = 22 * 60 * 60 * 1000;

  async function handleLeaveRoom(roomId: string) {
    pendingAction = 'leaveRoom';
    try {
      const api = serverConnectionManager.getClient(activeInstanceId).getAPI(createRoomCommandAPI);
      await api.leaveRoom(roomId);
    } catch (error) {
      toast.error(m['room.leave.failed']());
      console.error('Error leaving room:', error);
      closeModal();
      return;
    } finally {
      if (pendingAction === 'leaveRoom') pendingAction = undefined;
    }

    clearLastRoom(activeInstanceId);
    goto(resolve('/chat/[serverId]', { serverId: serverSegment }));
  }

  function handleRemoveServer() {
    // Removing a server no longer hits the API — server membership
    // is implicit on signup, so the action is purely a client-side disconnect:
    // forget the instance from the registry and route somewhere safe.
    const targetServerId = modalServerId;
    clearLastRoom(targetServerId);

    const leftInstanceId = targetServerId;
    serverRegistry.removeServer(leftInstanceId);

    if (leftInstanceId !== activeInstanceId) {
      closeModal();
      return;
    }

    // Land on the origin instance if it exists, otherwise root.
    const originId = serverRegistry.originServer?.id;
    if (originId && originId !== leftInstanceId) {
      goto(resolve('/chat/[serverId]', { serverId: serverIdToSegment(originId) }));
    } else {
      goto(resolve('/'));
    }
  }

  async function handleDeleteMessage(roomId: string, eventId: string) {
    pendingAction = 'deleteMessage';
    try {
      await getActiveMessageAPI().deleteMessage(roomId, eventId);
    } catch (error) {
      toast.error(m['room.message.delete_failed']());
      console.error('Error deleting message:', error);
      closeModal();
      return;
    } finally {
      if (pendingAction === 'deleteMessage') pendingAction = undefined;
    }
    notifyRoomMessageMutated({ roomId, eventId, reason: 'message-deleted' });
    toast.success(m['room.message.deleted']());
    closeModal();
  }

  async function handleDeleteLinkPreview(roomId: string, eventId: string, previewUrl: string) {
    pendingAction = 'deleteLinkPreview';
    try {
      await getActiveMessageAPI().deleteLinkPreview(roomId, eventId, previewUrl);
    } catch (error) {
      toast.error(m['room.link_preview.delete_failed']());
      console.error('Error deleting link preview:', error);
      closeModal();
      return;
    } finally {
      if (pendingAction === 'deleteLinkPreview') pendingAction = undefined;
    }
    notifyRoomMessageMutated({ roomId, eventId, reason: 'link-preview-deleted' });
    closeModal();
  }

  async function handleDeleteAttachment(roomId: string, eventId: string, attachmentId: string) {
    pendingAction = 'deleteAttachment';
    try {
      await getActiveMessageAPI().deleteAttachment(roomId, eventId, attachmentId);
    } catch (error) {
      toast.error(m['room.attachment.delete_failed']());
      console.error('Error deleting attachment:', error);
      closeModal();
      return;
    } finally {
      if (pendingAction === 'deleteAttachment') pendingAction = undefined;
    }
    notifyRoomMessageMutated({ roomId, eventId, reason: 'attachment-deleted' });
    closeModal();
  }

  async function refreshImageViewerUrls() {
    const currentModal = modal;
    if (
      currentModal?.type !== 'imageViewer' ||
      !currentModal.roomId ||
      !currentModal.eventId ||
      !currentModal.imageItems?.length
    ) {
      return;
    }
    const refreshRoomId = currentModal.roomId;
    const refreshEventId = currentModal.eventId;
    const freshUrls = await refreshAttachmentUrlsForAssets(
      getActiveAttachmentAPI(),
      refreshRoomId,
      currentModal.imageItems.map((item) => item.id).filter((id): id is string => !!id),
      LIGHTBOX_ATTACHMENT_IMAGE_REFRESH
    );
    if (freshUrls.size === 0) {
      return;
    }
    const latestModal = modal;
    if (
      latestModal?.type !== 'imageViewer' ||
      latestModal.roomId !== refreshRoomId ||
      latestModal.eventId !== refreshEventId ||
      !latestModal.imageItems?.length
    ) {
      return;
    }
    const imageItems = latestModal.imageItems
      .map((item) => {
        const refreshed = item.id ? freshUrls.get(item.id) : undefined;
        return {
          ...item,
          src: refreshed
            ? (assetUrlForServer(activeInstanceId, refreshed.thumbnailAssetUrl?.url) ?? '')
            : item.src,
          originalSrc: refreshed
            ? (assetUrlForServer(activeInstanceId, refreshed.assetUrl?.url) ?? undefined)
            : item.originalSrc
        };
      })
      .filter((item) => item.src !== '');
    if (imageItems.length === 0) {
      closeModal();
      return;
    }
    const currentImageId = latestModal.imageItems[latestModal.imageIndex ?? 0]?.id;
    const refreshedImageIndex = currentImageId
      ? imageItems.findIndex((item) => item.id === currentImageId)
      : -1;
    replaceState('', {
      ...page.state,
      modal: {
        ...latestModal,
        imageItems,
        imageIndex:
          refreshedImageIndex >= 0
            ? refreshedImageIndex
            : Math.min(latestModal.imageIndex ?? 0, imageItems.length - 1)
      }
    });
  }

  $effect(() => {
    if (modal?.type !== 'imageViewer') {
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

{#if modal?.type === 'createRoom'}
  <Dialog visible title={m['room.create.title']()} size="md" onclose={closeModal}>
    <p class="mb-4 text-muted">{m['room.create.description']()}</p>
    <CreateRoom onroomcreated={(roomId) => handleRoomCreated(roomId)} />
  </Dialog>
{:else if modal?.type === 'logout'}
  <SignOutDialog onclose={closeModal} />
{:else if modal?.type === 'aboutChatto'}
  <Dialog
    visible
    title={m['ui.tooltip.about']({ subject: 'Chatto' })}
    size="lg"
    onclose={closeModal}
  >
    <div class="flex flex-col items-center gap-4 text-sm">
      <div class="flex aspect-[2/1] w-full items-center justify-center">
        {#await loadSimulatedChattoWordmark() then { default: SimulatedChattoWordmark }}
          <SimulatedChattoWordmark contained />
        {/await}
      </div>

      <p class="text-muted tabular-nums">v{version}</p>

      <div class="flex flex-wrap items-center justify-center gap-x-5 gap-y-2">
        <a
          href="https://github.com/chattocorp/chatto"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-1.5 link"
        >
          <span class="iconify text-base mdi--github" aria-hidden="true"></span>
          <span>github.com/chattocorp/chatto</span>
          <span class="iconify text-sm mdi--open-in-new" aria-hidden="true"></span>
        </a>
        <a
          href="https://docs.chatto.run"
          target="_blank"
          rel="noopener noreferrer"
          class="inline-flex items-center gap-1.5 link"
        >
          <span class="iconify text-base mdi--book-open-page-variant-outline" aria-hidden="true"
          ></span>
          <span>docs.chatto.run</span>
          <span class="iconify text-sm mdi--open-in-new" aria-hidden="true"></span>
        </a>
      </div>
    </div>
  </Dialog>
{:else if modal?.type === 'leaveRoom' && modal.roomId}
  <ConfirmDialog
    title={m['room.leave.title']()}
    actionLabel={m['room.leave.action']()}
    actionIcon="iconify uil--sign-out-alt"
    loading={pendingAction === 'leaveRoom'}
    onconfirm={() => handleLeaveRoom(modal.roomId!)}
    onclose={closeModal}
  >
    {m['room.leave.prompt']({ room: modal.roomName ?? '' })}
  </ConfirmDialog>
{:else if modal?.type === 'removeServer'}
  <ConfirmDialog
    title={m['room.server.remove_title']()}
    actionLabel={m['room.server.remove_action']()}
    actionIcon="iconify uil--minus-circle"
    onconfirm={() => handleRemoveServer()}
    onclose={closeModal}
  >
    <p>{m['room.server.remove_prompt']({ server: modal.spaceName ?? '' })}</p>
    <p class="mt-3 text-sm text-muted">
      {m['room.server.remove_account_prefix']()}
      <a
        href={resolve('/chat/[serverId]/settings/account', {
          serverId: serverIdToSegment(modalServerId)
        })}
        class="link">{m['room.server.remove_account_link']()}</a
      >{m['room.server.remove_account_suffix']()}
    </p>
  </ConfirmDialog>
{:else if modal?.type === 'deleteMessage' && modal.roomId && modal.eventId}
  <ConfirmDialog
    title={m['room.message.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={pendingAction === 'deleteMessage'}
    onconfirm={() => handleDeleteMessage(modal.roomId!, modal.eventId!)}
    onclose={closeModal}
  >
    {m['room.message.delete_prompt']()}
  </ConfirmDialog>
{:else if modal?.type === 'deleteAttachment' && modal.roomId && modal.eventId && modal.attachmentId}
  <ConfirmDialog
    title={m['room.attachment.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={pendingAction === 'deleteAttachment'}
    onconfirm={() => handleDeleteAttachment(modal.roomId!, modal.eventId!, modal.attachmentId!)}
    onclose={closeModal}
  >
    {m['room.attachment.delete_prompt']()}
  </ConfirmDialog>
{:else if modal?.type === 'deleteLinkPreview' && modal.roomId && modal.eventId && modal.previewUrl}
  <ConfirmDialog
    title={m['room.link_preview.delete_title']()}
    actionLabel={m['common.delete']()}
    actionIcon="iconify uil--trash-alt"
    loading={pendingAction === 'deleteLinkPreview'}
    onconfirm={() => handleDeleteLinkPreview(modal.roomId!, modal.eventId!, modal.previewUrl!)}
    onclose={closeModal}
  >
    {m['room.link_preview.delete_prompt']()}
  </ConfirmDialog>
{:else if modal?.type === 'imageViewer' && modal.imageItems && modal.imageItems.length > 0}
  <ImageModal items={modal.imageItems} index={modal.imageIndex ?? 0} onclose={closeModal} />
{/if}
