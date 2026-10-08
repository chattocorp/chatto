<!-- @component
Owns one call card's responsive action layout and media controls. Callers supply
the card content and live menu target. Replacing a card releases its observers;
resizing or closing its menu leaves media observation with the mounted card.
-->
<script module lang="ts">
  import { ResponsiveActions } from '$lib/ui';
  import { CallPictureInPicture } from './CallPictureInPicture.svelte';

  /** Independent layout and media controllers shared by the card's action presentations. */
  export type CallCardControls = {
    actions: ResponsiveActions;
    media: CallPictureInPicture;
  };
</script>

<script lang="ts" generics="T">
  import type { UserMenuState } from '$lib/components/users/UserMenuState.svelte';
  import { untrack, type Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  let {
    menu,
    target,
    focusFallback,
    focusScope,
    children,
    ...attributes
  }: Omit<HTMLAttributes<HTMLDivElement>, 'children'> & {
    menu: UserMenuState<T & { controls: CallCardControls }>;
    /** Resolve current participant data when a context-menu gesture occurs. */
    target: () => T | null;
    focusFallback: () => HTMLElement | null | undefined;
    focusScope: () => HTMLElement | null | undefined;
    children: Snippet<[CallCardControls]>;
  } = $props();

  // The host menu is constant for this mounted card; target data stays live.
  const hostMenu = untrack(() => menu);
  const actions: ResponsiveActions = new ResponsiveActions({
    breakpointRem: 20,
    isMenuOpen: () => hostMenu.target?.controls === controls,
    dismissMenu: () => hostMenu.close(),
    focusFallback: () => focusFallback(),
    focusScope: () => focusScope()
  });
  const media = new CallPictureInPicture();
  const controls: CallCardControls = { actions, media };
  const triggerMenu = untrack(() =>
    hostMenu.trigger(() => {
      const current = target();
      return current === null ? null : { ...current, controls };
    })
  );
</script>

<div {...attributes} {@attach actions.observe} {@attach triggerMenu}>
  {@render children(controls)}
</div>
