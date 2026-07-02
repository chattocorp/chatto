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
  class="pointer-events-none fixed right-3 bottom-[calc(env(safe-area-inset-bottom,0px)+0.75rem)] left-3 z-50 flex flex-col items-stretch gap-2 sm:right-4 sm:bottom-[calc(env(safe-area-inset-bottom,0px)+1rem)] sm:left-auto sm:items-end"
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
    animation: toast-in 160ms cubic-bezier(0.2, 0, 0, 1);
    transform-origin: right bottom;
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translate3d(0.5rem, 0.25rem, 0) scale(0.98);
    }
    to {
      opacity: 1;
      transform: translate3d(0, 0, 0) scale(1);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .toast-enter {
      animation: toast-fade-in 120ms ease-out;
    }

    @keyframes toast-fade-in {
      from {
        opacity: 0;
      }
      to {
        opacity: 1;
      }
    }
  }
</style>
