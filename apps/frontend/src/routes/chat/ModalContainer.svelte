<script lang="ts">
  import { page } from '$app/state';
  import { chatModalKey, type ChatModal } from '$lib/modal';
  import AboutChattoModal from './modals/AboutChattoModal.svelte';
  import DeleteAttachmentModal from './modals/DeleteAttachmentModal.svelte';
  import DeleteLinkPreviewModal from './modals/DeleteLinkPreviewModal.svelte';
  import DeleteMessageModal from './modals/DeleteMessageModal.svelte';
  import ImageViewerModal from './modals/ImageViewerModal.svelte';
  import LeaveRoomModal from './modals/LeaveRoomModal.svelte';
  import RemoveServerModal from './modals/RemoveServerModal.svelte';
  import SignOutDialog from './SignOutDialog.svelte';

  const modal = $derived(page.state.modal);

  function closeModalFor(expectedModal: ChatModal) {
    return () => {
      if (page.state.modal === expectedModal) history.back();
    };
  }
</script>

{#if modal}
  {#key chatModalKey(modal)}
    {@const closeModal = closeModalFor(modal)}
    {#if modal.type === 'logout'}
      <SignOutDialog onclose={closeModal} />
    {:else if modal.type === 'aboutChatto'}
      <AboutChattoModal onclose={closeModal} />
    {:else if modal.type === 'leaveRoom'}
      <LeaveRoomModal {modal} onclose={closeModal} />
    {:else if modal.type === 'removeServer'}
      <RemoveServerModal {modal} onclose={closeModal} />
    {:else if modal.type === 'deleteMessage'}
      <DeleteMessageModal {modal} onclose={closeModal} />
    {:else if modal.type === 'deleteAttachment'}
      <DeleteAttachmentModal {modal} onclose={closeModal} />
    {:else if modal.type === 'deleteLinkPreview'}
      <DeleteLinkPreviewModal {modal} onclose={closeModal} />
    {:else if modal.type === 'imageViewer'}
      <ImageViewerModal {modal} onclose={closeModal} />
    {/if}
  {/key}
{/if}
