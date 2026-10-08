<!-- @component
Renders a header or menu PiP action from card-owned media and responsive layout state.
VideoThumbnail owns the observation attachment, independently of button visibility.
-->
<script lang="ts">
  import { CompactActionButton, MenuItem, MenuSection } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import type { CallCardControls } from './CallCard.svelte';
  let {
    controls,
    presentation = 'button',
    onToggle
  }: {
    controls: CallCardControls;
    presentation?: 'button' | 'menu';
    /** Close the selected menu after success; dismissal never stops PiP. */
    onToggle?: () => void;
  } = $props();

  const disabled = $derived(
    controls.media.pending ||
      !controls.media.available ||
      (!controls.media.ready && !controls.media.active)
  );

  async function toggle(event: MouseEvent): Promise<void> {
    const onSuccess = onToggle;
    if (await controls.media.togglePictureInPicture(event)) onSuccess?.();
  }
</script>

{#if controls.media.supported && presentation === 'menu'}
  <MenuSection>
    <MenuItem
      icon="icon-[mdi--picture-in-picture-bottom-right]"
      {disabled}
      busy={controls.media.pending}
      pressed={controls.media.active}
      dataTestid="call-menu-pip-button"
      onclick={toggle}
    >
      {m('voice.picture_in_picture')}
      {#snippet trailing()}
        {#if controls.media.active}
          <span class="iconify icon-[uil--check]" aria-hidden="true"></span>
        {/if}
      {/snippet}
    </MenuItem>
  </MenuSection>
{:else if controls.media.supported && !controls.actions.compact}
  <CompactActionButton
    label={m('voice.picture_in_picture')}
    aria-pressed={controls.media.active}
    {disabled}
    data-testid="call-feed-pip-button"
    onclick={toggle}
  >
    <span class="iconify icon-[mdi--picture-in-picture-bottom-right]" aria-hidden="true"></span>
  </CompactActionButton>
{/if}
