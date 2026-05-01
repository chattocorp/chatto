<script lang="ts">
  import type { ToastAction, ToastTone } from './toastState.svelte';

  let {
    tone,
    message,
    action,
    onDismiss
  }: {
    tone: ToastTone;
    message: string;
    action?: ToastAction;
    onDismiss: () => void;
  } = $props();

  const icons: Record<ToastTone, string> = {
    error: 'iconify mdi--alert-circle',
    success: 'iconify mdi--check-circle',
    info: 'iconify mdi--information',
    warning: 'iconify mdi--alert'
  };

  const iconColors: Record<ToastTone, string> = {
    error: 'text-red-500',
    success: 'text-green-500',
    info: 'text-blue-500',
    warning: 'text-amber-500'
  };

  function handleActionClick(e: MouseEvent) {
    e.stopPropagation();
    action?.onClick();
    onDismiss(); // Close toast after action is clicked
  }

  function handleKeyDown(e: KeyboardEvent) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onDismiss();
    }
  }
</script>

<!-- Using div instead of button to allow nesting the action button (nested buttons are invalid HTML) -->
<div
  class="flex max-w-96 min-w-64 cursor-pointer items-center gap-3 rounded-lg border border-border bg-surface px-4 py-3 text-left shadow-lg transition-colors hover:bg-surface-highlighted"
  onclick={onDismiss}
  onkeydown={handleKeyDown}
  role="button"
  tabindex="0"
  aria-label="Dismiss notification"
>
  <span class="{icons[tone]} {iconColors[tone]} size-5 shrink-0"></span>
  <span class="flex-1 text-sm text-text">{message}</span>
  {#if action}
    <button
      type="button"
      class="shrink-0 cursor-pointer rounded bg-primary px-3 py-1 text-xs font-medium text-white hover:bg-primary/80"
      onclick={handleActionClick}
    >
      {action.label}
    </button>
  {/if}
</div>
