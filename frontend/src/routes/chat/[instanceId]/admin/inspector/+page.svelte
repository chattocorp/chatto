<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { getCurrentUser } from '$lib/auth/currentUser.svelte';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { Panel } from '$lib/components/admin';
  import { PermissionInspectorPanel } from '$lib/components/rbac';
  import PaneHeader from '$lib/ui/PaneHeader.svelte';
  import PageTitle from '$lib/ui/PageTitle.svelte';

  const currentUser = getCurrentUser();
  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));

  const targetUserId = $derived(page.url.searchParams.get('userId') ?? currentUser.user?.id ?? '');
  const targetSpaceId = $derived(page.url.searchParams.get('spaceId') ?? null);
  const targetRoomId = $derived(page.url.searchParams.get('roomId') ?? null);

  let userInput = $state('');
  let spaceInput = $state('');
  let roomInput = $state('');

  $effect(() => {
    userInput = targetUserId;
  });
  $effect(() => {
    spaceInput = targetSpaceId ?? '';
  });
  $effect(() => {
    roomInput = targetRoomId ?? '';
  });

  function applyParams(newUserId: string, newSpaceId: string, newRoomId: string) {
    const params = new URLSearchParams();
    if (newUserId) params.set('userId', newUserId);
    if (newSpaceId) params.set('spaceId', newSpaceId);
    if (newRoomId) params.set('roomId', newRoomId);
    const base = resolve('/chat/[instanceId]/admin/inspector', { instanceId: instanceSegment });
    const search = params.toString();
    goto(search ? `${base}?${search}` : base, { replaceState: true, keepFocus: true });
  }

  const scopeLabel = $derived.by(() => {
    if (targetRoomId) return 'room scope';
    if (targetSpaceId) return 'space scope';
    return 'instance scope';
  });
</script>

<PageTitle title="Permission Inspector | Admin" />

<div class="flex min-h-0 min-w-0 flex-1 flex-col">
  <PaneHeader
    title="Permission Inspector"
    subtitle="Inspect any user's effective permissions and see why each was granted or denied"
    showMobileNav
  />

  <div class="flex flex-col gap-6 overflow-y-auto p-6">
    <Panel title="Inspect" icon="iconify uil--search">
      <p class="mb-4 text-sm text-muted">
        Pick a user and an optional space (and optional room within that space). Leave space and
        room empty for instance-level permissions.
      </p>
      <form
        class="flex flex-wrap items-end gap-4"
        onsubmit={(e) => {
          e.preventDefault();
          applyParams(userInput.trim(), spaceInput.trim(), roomInput.trim());
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
          <span class="mb-1 text-muted">Space ID (optional)</span>
          <input
            type="text"
            bind:value={spaceInput}
            placeholder="S…"
            class="input min-w-[18rem]"
          />
        </label>
        <label class="flex flex-col text-sm">
          <span class="mb-1 text-muted">Room ID (optional, requires space)</span>
          <input type="text" bind:value={roomInput} placeholder="R…" class="input min-w-[18rem]" />
        </label>
        <button type="submit" class="btn btn-primary cursor-pointer">Inspect</button>
      </form>
    </Panel>

    {#if targetUserId}
      <Panel title="Effective permissions" icon="iconify uil--lock-access">
        <p class="mb-4 text-sm text-muted">
          Showing {scopeLabel} for user <code>{targetUserId}</code>{#if targetSpaceId}
            in space <code>{targetSpaceId}</code>{/if}{#if targetRoomId}
            and room <code>{targetRoomId}</code>{/if}.
        </p>
        <PermissionInspectorPanel
          userId={targetUserId}
          spaceId={targetSpaceId}
          roomId={targetRoomId}
        />
      </Panel>
    {/if}
  </div>
</div>
