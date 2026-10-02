<!--
@component

Keeps every eligible server's Web Push subscription saved after the browser
grants notification permission. It never asks for permission: the header
action on the Notifications page does that.

A server that becomes eligible later, for example after it is added, is
registered at once. When the window gets focus and once an hour, the
component checks every server: it saves a subscription again when the last
save in this page is a day old or the browser replaced the subscription.
These refreshes keep the server-side subscriptions from expiring while the
device is in use.

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

  function refreshStaleSubscriptions(): void {
    void refreshPushSubscriptions();
  }

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (getPermission() !== 'granted' || targets.length === 0) return;

    void refreshPushSubscriptions();
  });
</script>

<svelte:window onfocus={refreshStaleSubscriptions} />
<Interval milliseconds={refreshCheckIntervalMs} ontick={refreshStaleSubscriptions} />
