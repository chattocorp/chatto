<!-- @component Link actions shared by touch preview sheets and entity menus. -->
<script lang="ts">
  import { MenuItem, MenuSection } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import { openRenderedLink } from '$lib/navigation/renderedLink';
  import { copyToClipboard } from '$lib/utils/copyToClipboard';

  let { href, onclose }: { href: string; onclose: () => void } = $props();
</script>

<MenuSection>
  <MenuItem
    icon="icon-[uil--external-link-alt]"
    onclick={() => {
      openRenderedLink(href);
      onclose();
    }}>{m('preview.open_link')}</MenuItem
  >
  <MenuItem
    icon="icon-[uil--copy]"
    onclick={() => {
      void copyToClipboard(href);
      onclose();
    }}>{m('room.message.actions.copy_link')}</MenuItem
  >
</MenuSection>
