<!-- @component
Shows a local diagram snapshot from shallow route state in the shared image viewer.
The modal container owns its lifetime and history-based dismissal.
-->
<script lang="ts">
  import type { RenderedImageViewerModalState } from '$lib/modal';
  import { m } from '$lib/i18n/messages';
  import { AttachmentModal, ZoomableImage } from '$lib/ui/attachments';
  import { svgDataUrl } from '$lib/svgAnimationFrame';

  let { modal, onclose }: { modal: RenderedImageViewerModalState; onclose: () => void } = $props();
  let failed = $state(false);
  // Keep the encoded copy only while the viewer is mounted, outside history state.
  const src = $derived(modal.svgSource === undefined ? modal.src : svgDataUrl(modal.svgSource));
</script>

<AttachmentModal
  filename="diagram.svg"
  contentType="image/svg+xml"
  description={modal.alt}
  downloadUrl={src}
  imageViewer
  error={failed ? m('room.attachment.html_viewer.preview_failed') : null}
  ondownload={(event) => event.stopPropagation()}
  {onclose}
>
  <ZoomableImage
    {src}
    alt={modal.alt}
    svgSource={modal.svgSource}
    onerror={() => (failed = true)}
  />
</AttachmentModal>
