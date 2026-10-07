<!--
@component

Keeps Web Push subscriptions current for the chat shell. This component has
no visible UI and never requests notification permission. The user enables
push with the checkbox in notification settings.

After the browser grants permission, the component keeps every eligible
server's subscription saved. A server that becomes eligible later, for example
after it is added, is registered at once. When the window gets focus and once
an hour, the component checks every server: it saves a subscription again when
the last save in this page is a day old, failed, or the browser replaced the
subscription. These refreshes keep the server-side subscriptions from expiring
while the device is in use.

Include this component once in the chat root.
-->
<script lang="ts">
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    getPermission,
    getPushRegistrationTargets,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';

  const refreshCheckIntervalMs = 60 * 60 * 1000;

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (getPermission() !== 'granted' || targets.length === 0) return;

    void refreshPushSubscriptions();
  });
</script>

<svelte:window onfocus={() => void refreshPushSubscriptions()} />
<Interval milliseconds={refreshCheckIntervalMs} ontick={() => void refreshPushSubscriptions()} />
