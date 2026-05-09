<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment, segmentToInstanceId } from '$lib/navigation';
  import { instanceRegistry } from '$lib/state/instance/registry.svelte';
  import { graphqlClientManager } from '$lib/state/instance/graphqlClient.svelte';

  // ModalContainer renders in chat/+layout.svelte (above [instanceId]),
  // so it cannot use getActiveInstance(). Derive instance from the URL params instead.
  const activeInstanceId = $derived(
    segmentToInstanceId(page.params.instanceId ?? '-') ?? instanceRegistry.originInstance?.id ?? ''
  );
  const instanceSegment = $derived(instanceIdToSegment(activeInstanceId));
  import Dialog from '$lib/ui/Dialog.svelte';
  import ConfirmDialog from '$lib/ui/ConfirmDialog.svelte';
  import CreateRoom from '$lib/CreateRoom.svelte';

  import ImageModal from '$lib/ui/ImageModal.svelte';

  import { graphql } from '$lib/gql';
  import { toast } from '$lib/ui/toast';
  import { clearLastRoom } from '$lib/storage/lastRoom';
  import { notifyLogout } from '$lib/auth/sessionChannel';

  /** Get the GraphQL client for the currently active instance (derived from URL). */
  function getActiveClient() {
    return graphqlClientManager.getClient(activeInstanceId).client;
  }

  function closeModal() {
    history.back();
  }

  function handleRoomCreated(roomId: string) {
    goto(resolve('/chat/[instanceId]/(chrome)/[roomId]', { instanceId: instanceSegment, roomId }));
  }

  let leavingRoom = $state(false);
  let leavingServer = $state(false);
  let deletingMessage = $state(false);
  let deletingLinkPreview = $state(false);
  let deletingAttachment = $state(false);

  async function handleLeaveRoom(roomId: string) {
    leavingRoom = true;
    const result = await getActiveClient().mutation(
        graphql(`
          mutation LeaveRoomFromModal($input: LeaveRoomInput!) {
            leaveRoom(input: $input)
          }
        `),
        { input: { roomId } }
      )
      .toPromise();
    leavingRoom = false;

    if (result.error) {
      toast.error('Failed to leave room');
      console.error('Error leaving room:', result.error);
      closeModal();
      return;
    }

    clearLastRoom(activeInstanceId);
    goto(resolve('/chat/[instanceId]', { instanceId: instanceSegment }));
  }

  async function handleLeaveServer() {
    // Post-#330 PR(a) "leave server" no longer hits the API — server membership
    // is implicit on signup, so the action is purely a client-side disconnect:
    // forget the instance from the registry and route somewhere safe.
    leavingServer = true;
    clearLastRoom(activeInstanceId);

    const leftInstanceId = activeInstanceId;
    instanceRegistry.removeInstance(leftInstanceId);

    // Land on the origin instance if it exists, otherwise root.
    const originId = instanceRegistry.originInstance?.id;
    if (originId && originId !== leftInstanceId) {
      goto(resolve('/chat/[instanceId]', { instanceId: instanceIdToSegment(originId) }));
    } else {
      goto(resolve('/'));
    }
    leavingServer = false;
  }

  async function handleDeleteMessage(roomId: string, eventId: string) {
    deletingMessage = true;
    const result = await getActiveClient().mutation(
        graphql(`
          mutation DeleteMessageFromModal($input: DeleteMessageInput!) {
            deleteMessage(input: $input)
          }
        `),
        { input: { roomId, eventId } }
      )
      .toPromise();
    deletingMessage = false;

    if (result.error) {
      toast.error('Failed to delete message');
      console.error('Error deleting message:', result.error);
    } else {
      toast.success('Message deleted');
    }
    closeModal();
  }

  async function handleDeleteLinkPreview(
    roomId: string,
    eventId: string,
    previewUrl: string
  ) {
    deletingLinkPreview = true;
    const result = await getActiveClient().mutation(
        graphql(`
          mutation DeleteLinkPreviewFromModal($input: DeleteLinkPreviewInput!) {
            deleteLinkPreview(input: $input)
          }
        `),
        { input: { roomId, eventId, url: previewUrl } }
      )
      .toPromise();
    deletingLinkPreview = false;

    if (result.error) {
      toast.error('Failed to delete link preview');
      console.error('Error deleting link preview:', result.error);
    }
    closeModal();
  }

  async function handleDeleteAttachment(
    roomId: string,
    eventId: string,
    attachmentId: string
  ) {
    deletingAttachment = true;
    const result = await getActiveClient().mutation(
        graphql(`
          mutation DeleteAttachmentFromModal($input: DeleteAttachmentInput!) {
            deleteAttachment(input: $input)
          }
        `),
        { input: { roomId, eventId, attachmentId } }
      )
      .toPromise();
    deletingAttachment = false;

    if (result.error) {
      toast.error('Failed to delete attachment');
      console.error('Error deleting attachment:', result.error);
    }
    closeModal();
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
</script>

{#if modalType === 'createRoom'}
  <Dialog visible title="Create a New Room" size="md" onclose={closeModal}>
    <p class="mb-4 text-muted">Rooms are conversations within your server.</p>
    <CreateRoom onroomcreated={(roomId) => handleRoomCreated(roomId)} />
  </Dialog>
{:else if modalType === 'logout'}
  <ConfirmDialog
    title="Sign Out"
    tone="info"
    actionLabel="Sign Out"
    actionIcon="iconify uil--signout"
    onconfirm={async () => {
      // Revoke the origin session cookie (if authenticated on origin)
      if (instanceRegistry.originInstance) {
        await fetch('/auth/logout', { method: 'POST' }).catch(() => {});
      }
      // Clear all registered instances and their state
      instanceRegistry.removeAll();
      notifyLogout();
      window.location.href = '/';
    }}
    onclose={closeModal}
  >
    This will disconnect all instances and sign you out. Your accounts on each instance are not affected.
  </ConfirmDialog>
{:else if modalType === 'leaveRoom' && roomId}
  <ConfirmDialog
    title="Leave Room"
    actionLabel="Leave Room"
    actionIcon="iconify uil--sign-out-alt"
    loading={leavingRoom}
    onconfirm={() => handleLeaveRoom(roomId)}
    onclose={closeModal}
  >
    Are you sure you want to leave <strong>#{roomName}</strong>? You can rejoin later.
  </ConfirmDialog>
{:else if modalType === 'leaveServer'}
  <ConfirmDialog
    title="Leave Server"
    actionLabel="Leave Server"
    actionIcon="iconify uil--sign-out-alt"
    loading={leavingServer}
    onconfirm={() => handleLeaveServer()}
    onclose={closeModal}
  >
    Are you sure you want to leave <strong>{spaceName}</strong>? You'll lose access to its rooms,
    and your client will forget about this server. You can re-add it later from the sidebar.
  </ConfirmDialog>
{:else if modalType === 'deleteMessage' && roomId && eventId}
  <ConfirmDialog
    title="Delete Message"
    actionLabel="Delete"
    actionIcon="iconify uil--trash-alt"
    loading={deletingMessage}
    onconfirm={() => handleDeleteMessage(roomId, eventId)}
    onclose={closeModal}
  >
    Are you sure you want to delete this message? This cannot be undone.
  </ConfirmDialog>
{:else if modalType === 'deleteAttachment' && roomId && eventId && attachmentId}
  <ConfirmDialog
    title="Delete Attachment"
    actionLabel="Delete"
    actionIcon="iconify uil--trash-alt"
    loading={deletingAttachment}
    onconfirm={() => handleDeleteAttachment(roomId, eventId, attachmentId)}
    onclose={closeModal}
  >
    Are you sure you want to delete this attachment? This cannot be undone.
  </ConfirmDialog>
{:else if modalType === 'deleteLinkPreview' && roomId && eventId && previewUrl}
  <ConfirmDialog
    title="Delete Link Preview"
    actionLabel="Delete"
    actionIcon="iconify uil--trash-alt"
    loading={deletingLinkPreview}
    onconfirm={() => handleDeleteLinkPreview(roomId, eventId, previewUrl)}
    onclose={closeModal}
  >
    Are you sure you want to remove this link preview? This cannot be undone.
  </ConfirmDialog>
{:else if modalType === 'imageViewer' && imageItems.length > 0}
  <ImageModal items={imageItems} index={imageIndex} onclose={closeModal} />
{/if}
