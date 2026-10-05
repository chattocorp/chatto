<!-- @component
Controls picture-in-picture for the video in the enclosing call media card.
Mount a new instance when the card's media track changes.
-->
<script lang="ts">
  import { CompactActionButton } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { toastError } from '$lib/utils/errorMessage';
  import { pictureInPictureVideo } from '$lib/state/callPictureInPicture';

  let supported = $state(false);
  let ready = $state(false);
  let active = $state(false);
  let pending = $state(false);
  let video: HTMLVideoElement | null = null;
  let mounted = false;

  /** Observe this stream, including its retained video after the tile mounts again. */
  function observeVideo(node: HTMLElement) {
    const element = node.closest('[data-call-media-card]')?.querySelector('video');
    if (!element) return;
    video = element;
    mounted = true;
    supported =
      typeof element.requestPictureInPicture === 'function' && document.pictureInPictureEnabled;

    function update() {
      ready = element!.readyState >= HTMLMediaElement.HAVE_METADATA && element!.videoWidth > 0;
      active = document.pictureInPictureElement === pictureInPictureVideo(element!);
    }

    const events = [
      'loadedmetadata',
      'loadeddata',
      'resize',
      'emptied',
      'enterpictureinpicture',
      'leavepictureinpicture'
    ];
    for (const event of events) element.addEventListener(event, update);
    document.addEventListener('enterpictureinpicture', update, true);
    document.addEventListener('leavepictureinpicture', update, true);
    update();

    return () => {
      mounted = false;
      for (const event of events) element.removeEventListener(event, update);
      document.removeEventListener('enterpictureinpicture', update, true);
      document.removeEventListener('leavepictureinpicture', update, true);
      video = null;
    };
  }

  async function toggle(event: MouseEvent) {
    event.stopPropagation();
    const element = video && pictureInPictureVideo(video);
    if (!element || pending || !supported || (!ready && !active)) return;
    pending = true;
    try {
      if (document.pictureInPictureElement === element) {
        await document.exitPictureInPicture();
      } else {
        await element.requestPictureInPicture();
        if (!mounted && document.pictureInPictureElement === element) {
          await document.exitPictureInPicture();
        }
      }
    } catch {
      if (mounted) toastError(null, m('voice.picture_in_picture_failed'));
    } finally {
      pending = false;
    }
  }
</script>

<div class="contents" {@attach observeVideo}>
  {#if supported}
    <CompactActionButton
      label={m('voice.picture_in_picture')}
      aria-pressed={active}
      disabled={pending || (!ready && !active)}
      data-testid="call-feed-pip-button"
      onclick={toggle}
    >
      <span class="iconify icon-[mdi--picture-in-picture-bottom-right]" aria-hidden="true"></span>
    </CompactActionButton>
  {/if}
</div>
