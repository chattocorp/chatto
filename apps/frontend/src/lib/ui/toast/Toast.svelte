<script lang="ts">
  import * as m from '$lib/i18n/messages';
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
    error: 'uil--times-circle',
    success: 'uil--check-circle',
    info: 'uil--info-circle',
    warning: 'uil--exclamation-triangle'
  };

  const iconColors: Record<ToastTone, string> = {
    error: 'text-error',
    success: 'text-success',
    info: 'text-accent',
    warning: 'text-warning'
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
  class="flex w-full max-w-96 min-w-0 cursor-pointer items-center gap-3 rounded-lg border border-text/10 bg-surface-100 px-3 py-2.5 text-left text-sm text-text shadow-xl transition-[background-color,scale] hover:bg-surface-200 active:scale-[0.99] sm:min-w-64"
  onclick={onDismiss}
  onkeydown={handleKeyDown}
  role="button"
  tabindex="0"
  aria-label={m['ui.toast.dismiss']()}
>
  <span class="flex size-6 shrink-0 items-center justify-center rounded bg-background">
    <span class={['iconify size-4', icons[tone], iconColors[tone]]} aria-hidden="true"></span>
  </span>
  <span class="min-w-0 flex-1 leading-snug break-words">{message}</span>
  {#if action}
    <button
      type="button"
      class="btn-secondary h-8 min-h-0 min-w-0 shrink-0 !rounded-md !px-3 !py-1 text-xs"
      onclick={handleActionClick}
    >
      {action.label}
    </button>
  {/if}
</div>
