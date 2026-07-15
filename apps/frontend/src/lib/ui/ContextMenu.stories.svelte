<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ContextMenu from './ContextMenu.svelte';

  const { Story } = defineMeta({
    title: 'UI/ContextMenu',
    component: ContextMenu,
    tags: ['autodocs'],
    parameters: {
      docs: {
        description: {
          component:
            'Responsive menu primitive: a top-layer floating menu on hover-capable devices and a bottom sheet on touch devices.'
        }
      }
    }
  });
</script>

<script lang="ts">
  let trigger: HTMLButtonElement;
  let open = $state(false);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);

  function openMenu() {
    const rect = trigger.getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }
</script>

<Story name="Floating menu" asChild>
  <button bind:this={trigger} type="button" class="btn-action" onclick={openMenu}>Open menu</button>

  {#if open}
    <ContextMenu
      {anchor}
      presentation="floating"
      ariaLabel="Example actions"
      onclose={() => (open = false)}
    >
      <div class="menu-section">
        <button type="button" class="menu-item" onclick={() => (open = false)}>
          <span class="sidebar-icon iconify uil--edit"></span>
          Edit
        </button>
        <button type="button" class="menu-item" onclick={() => (open = false)}>
          <span class="sidebar-icon iconify uil--copy"></span>
          Copy
        </button>
        <button type="button" class="menu-item text-danger" onclick={() => (open = false)}>
          <span class="sidebar-icon iconify uil--trash"></span>
          Delete
        </button>
      </div>
    </ContextMenu>
  {/if}
</Story>
