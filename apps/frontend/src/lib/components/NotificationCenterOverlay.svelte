<script lang="ts">
  import { notificationCenter, sidebarNav } from '$lib/state/globals.svelte';
  import BottomSheet from '$lib/ui/BottomSheet.svelte';
  import FloatingPopover from '$lib/ui/FloatingPopover.svelte';
  import { m } from '$lib/i18n/messages';
  import NotificationCenter from './NotificationCenter.svelte';
</script>

<svelte:window
  onkeydown={(event) => {
    if (event.key === 'Escape') notificationCenter.close();
  }}
  onresize={() => notificationCenter.close()}
/>

{#if sidebarNav.isMobile}
  <BottomSheet
    visible
    ariaLabel={m('ui.notifications')}
    onclose={() => notificationCenter.close()}
  >
    <div id="notification-center" class="overflow-hidden">
      <NotificationCenter
        class="min-h-40 max-h-[min(78dvh,44rem)]"
        onclose={() => notificationCenter.close()}
      />
    </div>
  </BottomSheet>
{:else if notificationCenter.anchor}
  <FloatingPopover
    id="notification-center"
    anchor={notificationCenter.anchor}
    role="dialog"
    ariaLabel={m('ui.notifications')}
    onclose={() => notificationCenter.close()}
    class="menu flex w-140 max-w-[90vw] flex-col gap-1"
  >
    <NotificationCenter
      class="min-h-40 max-h-[min(42rem,calc(100dvh-5rem))]"
      onclose={() => notificationCenter.close()}
    />
  </FloatingPopover>
{/if}
