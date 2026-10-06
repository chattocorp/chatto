<!-- @component
Wraps a user name or avatar in a room system event. When `onOpenUser` is set,
a click or right-click opens the user context menu, anchored at the wrapped
content. Without `onOpenUser`, the content renders unchanged.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { ClassValue } from 'svelte/elements';
  import type { UserAvatarUserView } from '@chatto/client/timeline/users';

  let {
    user,
    onOpenUser,
    class: className,
    children
  }: {
    user: UserAvatarUserView;
    onOpenUser?: (user: UserAvatarUserView, anchorRect: DOMRect) => void;
    class?: ClassValue;
    children: Snippet;
  } = $props();

  function openUser(event: MouseEvent & { currentTarget: HTMLButtonElement }) {
    onOpenUser?.(user, event.currentTarget.getBoundingClientRect());
  }
</script>

{#if onOpenUser}
  <button
    type="button"
    class={['cursor-pointer', className]}
    onclick={openUser}
    oncontextmenu={(event) => {
      event.preventDefault();
      event.stopPropagation();
      openUser(event);
    }}
  >
    {@render children()}
  </button>
{:else}
  {@render children()}
{/if}
