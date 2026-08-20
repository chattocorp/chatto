<script lang="ts">
  import { onMount } from 'svelte';

  // The promise represents ownership while the request is pending and after
  // its sentinel resolves. Clearing it invalidates stale completion callbacks;
  // release() still waits for a pending request so its sentinel cannot leak.
  let wakeLock: Promise<WakeLockSentinel> | null = null;

  function release(): void {
    const request = wakeLock;
    wakeLock = null;
    if (request)
      void request.then(
        (sentinel) => sentinel.release(),
        () => undefined
      );
  }

  function acquire(): void {
    if (document.visibilityState !== 'visible' || wakeLock || !('wakeLock' in navigator)) {
      return;
    }

    const request = navigator.wakeLock.request('screen');
    wakeLock = request;

    void request.then(
      (sentinel) => {
        // A release or newer request took ownership while this one was pending.
        if (wakeLock !== request) return;
        if (document.visibilityState !== 'visible') {
          release();
          return;
        }

        sentinel.addEventListener(
          'release',
          () => {
            if (wakeLock === request) wakeLock = null;
          },
          { once: true }
        );
      },
      () => {
        if (wakeLock === request) wakeLock = null;
      }
    );
  }

  function handleVisibilityChange(): void {
    if (document.visibilityState === 'visible') {
      acquire();
    } else {
      release();
    }
  }

  onMount(() => {
    acquire();
    return release;
  });
</script>

<!--
@component
Keeps the display awake for this component's mounted lifetime.

Render the component only while a wake lock is wanted. Unsupported or rejected
requests are intentionally ignored. Because browsers release screen wake locks
when a document becomes hidden, the component requests a new lock when the
document becomes visible again.
-->
<svelte:document onvisibilitychange={handleVisibilityChange} />
