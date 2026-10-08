<!-- @component
Renders the inline PiP action from card-owned media and responsive layout state.
VideoThumbnail owns the observation attachment, independently of button visibility.
-->
<script lang="ts">
  import { CompactActionButton } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import type { CallCardControls } from './CallCard.svelte';
  let { controls }: { controls: CallCardControls } = $props();
</script>

{#if controls.media.supported && !controls.actions.compact}
  <CompactActionButton
    label={m('voice.picture_in_picture')}
    aria-pressed={controls.media.active}
    disabled={controls.media.pending || (!controls.media.ready && !controls.media.active)}
    data-testid="call-feed-pip-button"
    onclick={controls.media.togglePictureInPicture}
  >
    <span class="iconify icon-[mdi--picture-in-picture-bottom-right]" aria-hidden="true"></span>
  </CompactActionButton>
{/if}
