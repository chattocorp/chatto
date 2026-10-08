<!-- @component
Story fixture for responsive action placement, menu dismissal, and focus recovery.
-->
<script lang="ts">
  import { ResponsiveActions } from './ResponsiveActions.svelte';
  import CompactActionButton from './CompactActionButton.svelte';
  import ContextMenu from './ContextMenu.svelte';
  import MenuItem from './MenuItem.svelte';
  import MenuSection from './MenuSection.svelte';
  import { Button } from './form';
  import { untrack } from 'svelte';

  let { width = '20rem', breakpointRem = 20 }: { width?: string; breakpointRem?: number } =
    $props();
  let size = $state<string | null>(null);
  let open = $state(false);
  let visible = $state(true);
  let notifications = $state(false);
  let anchor = $state<{ top: number; bottom: number; left: number } | null>(null);
  let scope: HTMLElement;
  let fallback: HTMLButtonElement;
  const actions = new ResponsiveActions({
    breakpointRem: untrack(() => breakpointRem),
    isMenuOpen: () => open,
    dismissMenu: close,
    focusFallback: () => fallback,
    focusScope: () => scope
  });

  function attachScope(element: HTMLElement) {
    scope = element;
  }

  function attachFallback(element: HTMLButtonElement) {
    fallback = element;
  }

  function close() {
    open = false;
    void actions.restoreFocus();
  }

  function openMenu(event: MouseEvent) {
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    anchor = { top: rect.top, bottom: rect.bottom, left: rect.left };
    open = true;
  }
</script>

<section {@attach attachScope} class="flex flex-col items-start gap-4">
  <div class="flex gap-2">
    <Button variant="secondary" onclick={() => (size = `${breakpointRem - 1}rem`)}
      >Narrow card</Button
    >
    <Button variant="secondary" onclick={() => (size = `${breakpointRem}rem`)}>Wide card</Button>
    <button
      {@attach attachFallback}
      type="button"
      class="btn-action"
      onclick={() => (visible = !visible)}
    >
      {visible ? 'Remove card' : 'Restore card'}
    </button>
  </div>
  {#if visible}
    <div
      class="flex min-w-0 items-center gap-2 shell-surface rounded-xl p-2"
      style:width={size ?? width}
      {@attach actions.observe}
    >
      <span class="min-w-0 flex-1 truncate font-semibold">Quarterly report</span>
      <div class="contents" {@attach actions.inline}>
        {#if !actions.compact}
          <CompactActionButton
            label="Notifications"
            aria-pressed={notifications}
            onclick={() => (notifications = !notifications)}
          >
            <span class="iconify icon-[uil--bell]" aria-hidden="true"></span>
          </CompactActionButton>
        {/if}
      </div>
      <CompactActionButton
        label="Document actions"
        aria-expanded={open}
        aria-haspopup="dialog"
        onclick={openMenu}
        {@attach actions.trigger}
      >
        <span class="iconify icon-[uil--ellipsis-v]" aria-hidden="true"></span>
      </CompactActionButton>
    </div>
  {/if}
  {#if open}
    <ContextMenu {anchor} role="dialog" ariaLabel="Document actions" onclose={close}>
      <MenuSection>
        {#if actions.compact}
          <MenuItem
            icon="icon-[uil--bell]"
            pressed={notifications}
            onclick={() => (notifications = !notifications)}>Notifications</MenuItem
          >
        {/if}
        <MenuItem icon="icon-[uil--copy]" onclick={close}>Copy link</MenuItem>
      </MenuSection>
    </ContextMenu>
  {/if}
</section>
