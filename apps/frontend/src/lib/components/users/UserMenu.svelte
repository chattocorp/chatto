<!--
@component

Lazy renderer for a host's user menu. The controller owns placement and
dismissal; the host supplies live user data, permissions, and available actions.
One renderer can serve a complete list without one menu instance per row.
-->
<script module lang="ts">
  export type UserContextMenuLoader = () => Promise<
    typeof import('$lib/components/menus/UserContextMenu.svelte')
  >;

  const loadUserContextMenu: UserContextMenuLoader = () =>
    import('$lib/components/menus/UserContextMenu.svelte');
</script>

<script lang="ts" generics="T">
  import type { ComponentProps } from 'svelte';
  import type UserContextMenu from '$lib/components/menus/UserContextMenu.svelte';
  import type { UserMenuState } from './UserMenuState.svelte';
  import { ContextMenu, LoadingFog, LoadRetry } from '$lib/ui';
  import { m } from '$lib/i18n/messages';

  type MenuProps = ComponentProps<typeof UserContextMenu>;

  let {
    state: menu,
    user,
    loader = loadUserContextMenu,
    ...actions
  }: {
    state: UserMenuState<T>;
    user: MenuProps['user'] | null | undefined;
    loader?: UserContextMenuLoader;
  } & Omit<MenuProps, 'user' | 'anchorRect' | 'position' | 'presentation' | 'onClose'> = $props();

  let attempt = $state(0);
  function load(_attempt: number) {
    return loader();
  }
</script>

{#snippet fallback(failed = false)}
  <ContextMenu
    anchor={menu.selection?.anchorRect}
    position={menu.selection?.position}
    presentation={menu.selection?.presentation}
    role={failed ? 'alertdialog' : 'dialog'}
    ariaLabel={m(failed ? 'common.error.generic' : 'common.loading')}
    onclose={menu.close}
  >
    {#if failed}
      <LoadRetry onretry={() => (attempt += 1)} />
    {:else}
      <LoadingFog class="m-2 h-28 w-64 max-w-full" />
    {/if}
  </ContextMenu>
{/snippet}

{#if menu.selection && user}
  {#await load(attempt)}
    {@render fallback()}
  {:then { default: Menu }}
    <Menu
      {user}
      {...actions}
      anchorRect={menu.selection.anchorRect}
      position={menu.selection.position}
      presentation={menu.selection.presentation}
      onClose={menu.close}
    />
  {:catch}
    {@render fallback(true)}
  {/await}
{/if}
