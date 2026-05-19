<!--
@component

Displays the current (server-scoped) user at the bottom of the secondary
sidebar. Shows the avatar with presence and the live display name, and links
to the user settings page for the active server.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { getLiveDisplayName } from '$lib/state/userProfiles.svelte';
  import UserAvatar from './UserAvatar.svelte';

  const activeServerId = $derived(getActiveServer());
  const serverSegment = $derived(serverIdToSegment(activeServerId));
  const activeServerUser = $derived(
    serverRegistry.tryGetStore(activeServerId)?.currentUser.user
  );

  const displayName = $derived(
    activeServerUser
      ? getLiveDisplayName(
          activeServerUser.id,
          activeServerUser.displayName || activeServerUser.login
        )
      : ''
  );

  const login = $derived(activeServerUser?.login ?? '');
  const showLogin = $derived(!!login && login !== displayName);

  const settingsHref = $derived(
    resolve('/chat/[serverId]/(chrome)/settings', { serverId: serverSegment })
  );

  const onSettings = $derived(page.url.pathname.startsWith(settingsHref));
</script>

{#if activeServerUser}
  <a
    href={settingsHref}
    title="User Settings"
    class={[
      'flex shrink-0 cursor-pointer items-center gap-3 border-t border-border px-3 py-3 hover:bg-surface-100',
      onSettings && 'bg-surface-100'
    ]}
  >
    <UserAvatar user={activeServerUser} size="md" />
    <div class="flex min-w-0 flex-1 flex-col leading-tight">
      <span class="truncate text-sm font-semibold">{displayName}</span>
      {#if showLogin}
        <span class="truncate text-xs text-muted">@{login}</span>
      {/if}
    </div>
    <span class="iconify shrink-0 text-muted uil--setting"></span>
  </a>
{/if}
