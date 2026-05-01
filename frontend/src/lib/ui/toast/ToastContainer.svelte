<script lang="ts">
  import { getToasts, toast } from './toastState.svelte';
  import Toast from './Toast.svelte';

  const toasts = $derived(getToasts());
</script>

<!--
  role=status + aria-live=polite makes screen readers announce toasts as
  they appear. We default to polite (rather than assertive) so toasts
  don't interrupt the user mid-sentence; error toasts are still
  announced, just at the next natural break.
-->
<div
  class="fixed right-4 bottom-4 z-50 flex flex-col gap-2 pointer-events-none"
  role="status"
  aria-live="polite"
  aria-atomic="false"
>
  {#each toasts as t (t.id)}
    <div class="toast-enter pointer-events-auto">
      <Toast
        tone={t.tone}
        message={t.message}
        action={t.action}
        onDismiss={() => toast.remove(t.id)}
      />
    </div>
  {/each}
</div>

<style>
  .toast-enter {
    animation: slide-in 150ms ease-out;
  }

  @keyframes slide-in {
    from {
      opacity: 0;
      transform: translateX(100%);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }
</style>
