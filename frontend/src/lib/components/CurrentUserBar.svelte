<!--
@component

Displays the current (server-scoped) user at the bottom of the secondary
sidebar. Shows the avatar with presence and the live display name, and links
to the user settings page for the active server.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
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
    resolve('/chat/[serverId]/settings', { serverId: serverSegment })
  );
</script>

{#if activeServerUser}
  <div class="shrink-0 p-2">
    <div class="flex items-center gap-3 rounded-xl bg-surface py-1 pr-3 pl-1">
      <UserAvatar user={activeServerUser} size="md" />
      <div class="flex min-w-0 flex-1 flex-col leading-tight">
        <span class="truncate text-sm font-semibold">{displayName}</span>
        {#if showLogin}
          <span class="truncate text-xs text-muted">@{login}</span>
        {/if}
      </div>
      <a
        href={settingsHref}
        title="User Settings"
        class="iconify shrink-0 cursor-pointer text-muted uil--setting hover:text-text"
      ></a>
    </div>
  </div>
{/if}
