<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FloatingPopover from './FloatingPopover.svelte';

  const { Story } = defineMeta({
    title: 'UI/Floating popover',
    component: FloatingPopover,
    tags: ['autodocs']
  });
</script>

<script lang="ts">
  let trigger: HTMLButtonElement;
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let open = $state(false);

  function toggle() {
    if (open) {
      open = false;
      return;
    }

    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }

  function captureTrigger(element: HTMLButtonElement) {
    trigger = element;
  }
</script>

<Story name="Anchored menu" asChild>
  <div class="flex min-h-52 items-center justify-center rounded-lg bg-background p-6">
    <button
      {@attach captureTrigger}
      type="button"
      class="btn-secondary"
      aria-expanded={open}
      onclick={toggle}
    >
      Open menu
    </button>

    {#if open}
      <FloatingPopover
        {anchor}
        role="menu"
        ariaLabel="Example menu"
        class="menu min-w-44 p-1 shadow-lg"
        onclose={() => (open = false)}
      >
        <button type="button" class="menu-item" role="menuitem">Rename</button>
        <button type="button" class="menu-item" role="menuitem">Move to folder</button>
        <button type="button" class="menu-item text-danger" role="menuitem">Archive</button>
      </FloatingPopover>
    {/if}
  </div>
</Story>
