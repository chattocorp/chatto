<script lang="ts">
  import { goto } from '$app/navigation';
  import { getActiveServerSpaceId } from '$lib/state/activeServer.svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { Panel } from '$lib/components/admin';
  import { PermissionInspectorPanel } from '$lib/components/rbac';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const currentUser = getCurrentUser();
  const getInstanceId = getActiveServer();
  const instanceSegment = $derived(serverIdToSegment(getInstanceId()));
  const spaceId = $derived(getActiveServerSpaceId()());

  const targetUserId = $derived(page.url.searchParams.get('userId') ?? currentUser.user?.id ?? '');
  const roomId = $derived(page.url.searchParams.get('roomId') ?? null);

  let userInput = $state('');
  let roomInput = $state('');

  $effect(() => {
    userInput = targetUserId;
  });
  $effect(() => {
    roomInput = roomId ?? '';
  });

  function applyParams(newUserId: string, newRoomId: string) {
    const params = new URLSearchParams();
    if (newUserId) params.set('userId', newUserId);
    if (newRoomId) params.set('roomId', newRoomId);
    const base = resolve('/chat/[serverId]/(chrome)/server-admin/inspector', {
      serverId: instanceSegment,
    });
    const search = params.toString();
    goto(search ? `${base}?${search}` : base, { replaceState: true, keepFocus: true });
  }
</script>

<PageTitle title="Permission Inspector | Space Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Permission Inspector"
    subtitle="Inspect a user's effective permissions in this space"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Panel title="Inspect" icon="iconify uil--search">
      <form
        class="flex flex-wrap items-end gap-4"
        onsubmit={(e) => {
          e.preventDefault();
          applyParams(userInput.trim(), roomInput.trim());
        }}
      >
        <label class="flex flex-col text-sm">
          <span class="mb-1 text-muted">User ID</span>
          <input
            type="text"
            bind:value={userInput}
            placeholder="U…"
            class="input min-w-[18rem]"
          />
        </label>
        <label class="flex flex-col text-sm">
          <span class="mb-1 text-muted">Room ID (optional)</span>
          <input type="text" bind:value={roomInput} placeholder="R…" class="input min-w-[18rem]" />
        </label>
        <button type="submit" class="btn btn-primary cursor-pointer">Inspect</button>
      </form>
    </Panel>

    {#if targetUserId}
      <Panel title="Effective permissions" icon="iconify uil--lock-access">
        <p class="mb-4 text-sm text-muted">
          {#if roomId}
            Showing room-scoped permissions for user <code>{targetUserId}</code> in this space and
            room <code>{roomId}</code>.
          {:else}
            Showing space-scoped permissions for user <code>{targetUserId}</code>.
          {/if}
        </p>
        <PermissionInspectorPanel userId={targetUserId} {roomId} />
      </Panel>
    {/if}
  </div>
</div>
