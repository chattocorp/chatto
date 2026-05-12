<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { graphql } from '$lib/gql';
  import { resolve } from '$app/paths';
  import { serverIdToSegment } from '$lib/navigation';
  import { getActiveServer } from '$lib/state/activeServer.svelte';
  import { useConnection } from '$lib/state/server/connection.svelte';
  import { serverRegistry } from '$lib/state/server/registry.svelte';
  import { toast } from '$lib/ui/toast';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import Room from './Room.svelte';

  let { data, children } = $props();

  const connection = useConnection();
  const serverSegment = $derived(serverIdToSegment(getActiveServer()));
  let { roomId } = $derived(data);

  // Wait for the active server's merged rooms store (channels + DMs) to
  // settle before letting children mount. Without this, a freshly-loaded
  // room page can fire queries against the URL roomId before the store has
  // decided whether the room exists, briefly showing the not-found redirect.
  const roomsStore = $derived(serverRegistry.getStore(getActiveServer()).rooms);
  const ready = $derived(!roomsStore.isInitialLoading);

  // Get threadId from URL params (only set when on the [threadId] route)
  let threadId = $derived(page.params.threadId);

  // Detect if we're in room settings mode or message link mode
  const isSettingsMode = $derived(page.url.pathname.includes('/settings'));
  const isMessageLinkMode = $derived(/\/m\/[^/]+$/.test(page.url.pathname));

  // Room settings data with proper loading state
  let roomSettingsData = $state<{ name: string } | null>(null);
  let roomSettingsLoading = $state(false);

  // Load room data for settings mode with race condition protection
  $effect(() => {
    if (!isSettingsMode || !ready || !roomId) {
      roomSettingsData = null;
      return;
    }

    // Capture current roomId to detect if it changes during async operation
    const currentRoomId = roomId;

    roomSettingsLoading = true;

    (async () => {
      try {
        const resp = await connection().client.query(
          graphql(`
            query GetRoomForSettings($roomId: ID!) {
              room(roomId: $roomId) {
                id
                name
              }
              server {
                viewerCanManageRooms
              }
            }
`),
          { roomId: currentRoomId }
        );

        // Abort if roomId changed during the request
        if (roomId !== currentRoomId) {
          return;
        }

        roomSettingsLoading = false;

        if (!resp.data?.room) {
          roomSettingsData = null;
          return;
        }

        // Check permission
        if (!resp.data.server?.viewerCanManageRooms) {
          toast.error('You do not have permission to manage this room');
          goto(
            resolve('/chat/[serverId]/(chrome)/[roomId]', {
              serverId: serverSegment,
              roomId: currentRoomId
            }),
            { replaceState: true }
          );
          return;
        }

        roomSettingsData = {
          name: resp.data.room.name
        };
      } catch {
        if (roomId === currentRoomId) {
          roomSettingsLoading = false;
          roomSettingsData = null;
        }
      }
    })();
  });

  // Settings navigation items
  const settingsNavItems = $derived(
    ready && roomId
      ? [
          {
            href: resolve('/chat/[serverId]/(chrome)/[roomId]/settings', {
              serverId: serverSegment,
              roomId
            }),
            label: 'Dashboard',
            icon: 'iconify uil--dashboard'
          },
          {
            href: resolve('/chat/[serverId]/(chrome)/[roomId]/settings/general', {
              serverId: serverSegment,
              roomId
            }),
            label: 'General',
            icon: 'iconify uil--setting'
          },
          {
            href: resolve('/chat/[serverId]/(chrome)/[roomId]/settings/permissions', {
              serverId: serverSegment,
              roomId
            }),
            label: 'Roles',
            icon: 'iconify uil--shield'
          }
        ]
      : []
  );

  // Check if a settings nav item is active
  // Note: `items` parameter is required by SidebarNav's isActive callback signature
  function isSettingsNavActive(
    href: string,
    _items: { href: string; label: string; icon: string }[]
  ): boolean {
    if (!ready || !roomId) return false;
    const settingsBase = resolve('/chat/[serverId]/(chrome)/[roomId]/settings', {
      serverId: serverSegment,
      roomId
    });
    if (href === settingsBase) {
      return page.url.pathname === settingsBase;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

{#if ready && roomId}
  {#if isMessageLinkMode}
    <!-- Message link resolver: renders +page.svelte which fetches + redirects -->
    {@render children?.()}
  {:else if isSettingsMode}
    <!-- Room Settings Mode: sidebar + content in flex-row -->
    <div class="flex min-h-0 min-w-0 flex-1 flex-row">
      <SecondarySidebar>
        <SidebarNav
          title={roomSettingsLoading
            ? 'Loading…'
            : roomSettingsData?.name
              ? `# ${roomSettingsData.name}`
              : 'Room'}
          items={settingsNavItems}
          backHref={resolve('/chat/[serverId]/(chrome)/[roomId]', {
            serverId: serverSegment,
            roomId
          })}
          backLabel="Back to Room"
          isActive={isSettingsNavActive}
        />
      </SecondarySidebar>

      <div class="flex min-h-0 min-w-0 flex-1 flex-col">
        {@render children?.()}
      </div>
    </div>
  {:else}
    <!--
			Room is rendered in the layout so it stays mounted when navigating
			between room and thread URLs. This prevents unnecessary reloads.
		-->
    <Room {roomId} {threadId} />
  {/if}
{/if}
