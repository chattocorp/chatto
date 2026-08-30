<script lang="ts">
  import ContextMenu from './ContextMenu.svelte';
  import MenuItem from './MenuItem.svelte';
  import MenuSection from './MenuSection.svelte';

  let {
    presentation = 'floating',
    containerRole = 'menu'
  }: {
    presentation?: 'floating' | 'sheet';
    containerRole?: 'menu' | 'dialog';
  } = $props();

  let clickCount = $state(0);
</script>

<output data-testid="click-count">{clickCount}</output>

<ContextMenu
  position={{ x: 24, y: 32 }}
  {presentation}
  role={containerRole}
  ariaLabel="Example actions"
  onclose={() => {}}
>
  <MenuSection ariaLabel="Primary actions">
    <MenuItem icon="icon-[uil--copy]" dataTestid="icon-button" onclick={() => (clickCount += 1)}>
      Copy identifier
    </MenuItem>
    <MenuItem dataTestid="text-button">Text-only action</MenuItem>
    <MenuItem href="/settings" dataTestid="link-item">Open settings</MenuItem>
    <MenuItem
      href="/blocked"
      disabled
      tone="danger"
      dataTestid="disabled-link"
      onclick={() => (clickCount += 1)}
    >
      Blocked action
    </MenuItem>
    <MenuItem role="menuitemradio" checked selected dataTestid="custom-item">
      {#snippet leading()}<span>🙂</span>{/snippet}
      {#snippet trailing()}
        <span class="iconify icon-[uil--check]"></span>
      {/snippet}
      A custom item with a label that can wrap onto another line
    </MenuItem>
  </MenuSection>
</ContextMenu>
