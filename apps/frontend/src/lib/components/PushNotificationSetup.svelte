<!--
@component

Takes care of Web Push without any Chatto UI.

While the browser's notification permission is unset and a signed-in server
supports push, the component asks the browser for permission once: at once
when the page still has user activation, for example right after sign-in,
otherwise on the user's next click or key press. Firefox and Safari ignore a
permission request outside a user interaction. The browser and the user decide
whether to allow, block, or dismiss the request.

After the browser grants permission, the component keeps every eligible
server's subscription saved. A server that becomes eligible later, for example
after it is added, is registered at once. When the window gets focus and once
an hour, the component checks every server: it saves a subscription again when
the last save in this page is a day old or the browser replaced the
subscription. These refreshes keep the server-side subscriptions from expiring
while the device is in use.

Include this component once in the chat root.
-->
<script lang="ts">
  import Interval from '$lib/lifecycle/Interval.svelte';
  import {
    enablePushOnAllServers,
    getPermission,
    getPushRegistrationTargets,
    refreshPushSubscriptions
  } from '$lib/notifications/pushNotifications';

  const refreshCheckIntervalMs = 60 * 60 * 1000;

  /** Whether this mount already asked, or decided to ask, for permission. */
  let permissionRequested = false;
  /** Whether the permission request waits for the next user interaction. */
  let awaitingInteraction = $state(false);

  function refreshStaleSubscriptions(): void {
    void refreshPushSubscriptions();
  }

  /**
   * Whether the page has user activation. Browsers without the User
   * Activation API, such as Firefox before 120, count only a trusted click or
   * a trusted press of a printable key or Enter: modifier keys and Escape do
   * not grant activation there.
   */
  function hasUserActivation(event?: Event): boolean {
    if (navigator.userActivation) return navigator.userActivation.isActive;
    if (event?.isTrusted !== true) return false;
    if (event instanceof KeyboardEvent) return event.key.length === 1 || event.key === 'Enter';
    return true;
  }

  function requestOnInteraction(event: Event): void {
    // Keys such as Escape or Shift do not grant user activation; wait for one that does.
    if (!awaitingInteraction || !hasUserActivation(event)) return;
    awaitingInteraction = false;
    if (getPermission() === 'default') void enablePushOnAllServers();
  }

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (permissionRequested || getPermission() !== 'default' || targets.length === 0) return;

    permissionRequested = true;
    if (hasUserActivation()) {
      void enablePushOnAllServers();
    } else {
      awaitingInteraction = true;
    }
  });

  $effect(() => {
    const targets = getPushRegistrationTargets();
    if (getPermission() !== 'granted' || targets.length === 0) return;

    void refreshPushSubscriptions();
  });
</script>

<svelte:window
  onfocus={refreshStaleSubscriptions}
  onclickcapture={requestOnInteraction}
  onkeydowncapture={requestOnInteraction}
/>
<Interval milliseconds={refreshCheckIntervalMs} ontick={refreshStaleSubscriptions} />
