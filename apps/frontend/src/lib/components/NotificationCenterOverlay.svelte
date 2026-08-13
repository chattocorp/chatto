<script lang="ts">
  import { notificationCenter } from '$lib/state/globals.svelte';
  import { Palette } from '$lib/ui';
  import { m } from '$lib/i18n/messages';
  import NotificationCenter from './NotificationCenter.svelte';
</script>

<Palette
  id="notification-center"
  visible={notificationCenter.visible}
  presentation="anchored"
  anchor={notificationCenter.anchor}
  ariaLabel={m('ui.notifications')}
  onclose={() => notificationCenter.close()}
>
  <NotificationCenter
    class="max-h-[min(42rem,calc(100dvh-5rem))] min-h-40"
    onclose={() => notificationCenter.close()}
  />
</Palette>
