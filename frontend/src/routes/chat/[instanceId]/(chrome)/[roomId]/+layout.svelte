<script lang="ts">
  import { page } from '$app/state';
  import { goto } from '$app/navigation';
  import { graphql } from '$lib/gql';
  import { resolve } from '$app/paths';
  import { instanceIdToSegment } from '$lib/navigation';
  import { getActiveInstance } from '$lib/state/activeInstance.svelte';
  import { useConnection } from '$lib/state/instance/connection.svelte';
  import { useEffectiveSpaceId } from '$lib/hooks';
  import { toast } from '$lib/ui/toast';
  import SecondarySidebar from '$lib/components/SecondarySidebar.svelte';
  import SidebarNav from '$lib/components/SidebarNav.svelte';
  import Room from './Room.svelte';

  // Get spaceId and roomId from the parent layout's data
  let { data, children } = $props();

  const connection = useConnection();
  const getInstanceId = getActiveInstance();
  const instanceSegment = $derived(instanceIdToSegment(getInstanceId()));
  let { roomId } = $derived(data);

  // The URL only carries roomId; useEffectiveSpaceId picks the right
  // underlying space (primary for channels, hidden DM space for DMs) from
  // the merged SpaceRoomsStore. Returns null while the store is loading —
  // the `{#if spaceId && roomId}` template gate below holds rendering.
  const effective = useEffectiveSpaceId(() => roomId);
  const spaceId = $derived(effective.current);

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
    if (!isSettingsMode || !spaceId || !roomId) {
      roomSettingsData = null;
      return;
    }

    // Capture current IDs to detect if they change during async operation
    const currentSpaceId = spaceId;
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

        // Abort if IDs changed during the request
        if (spaceId !== currentSpaceId || roomId !== currentRoomId) {
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
            resolve('/chat/[instanceId]/(chrome)/[roomId]', {
              instanceId: instanceSegment,
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
        if (spaceId === currentSpaceId && roomId === currentRoomId) {
          roomSettingsLoading = false;
          roomSettingsData = null;
        }
      }
    })();
  });

  // Settings navigation items
  const settingsNavItems = $derived(
    spaceId && roomId
      ? [
          {
            href: resolve('/chat/[instanceId]/(chrome)/[roomId]/settings', {
              instanceId: instanceSegment,
              roomId
            }),
            label: 'Dashboard',
            icon: 'iconify uil--dashboard'
          },
          {
            href: resolve('/chat/[instanceId]/(chrome)/[roomId]/settings/general', {
              instanceId: instanceSegment,
              roomId
            }),
            label: 'General',
            icon: 'iconify uil--setting'
          },
          {
            href: resolve('/chat/[instanceId]/(chrome)/[roomId]/settings/permissions', {
              instanceId: instanceSegment,
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
    if (!spaceId || !roomId) return false;
    const settingsBase = resolve('/chat/[instanceId]/(chrome)/[roomId]/settings', {
      instanceId: instanceSegment,
      roomId
    });
    if (href === settingsBase) {
      return page.url.pathname === settingsBase;
    }
    return page.url.pathname.startsWith(href);
  }
</script>

{#if spaceId && roomId}
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
          backHref={resolve('/chat/[instanceId]/(chrome)/[roomId]', {
            instanceId: instanceSegment,
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
