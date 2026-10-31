<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import { toastError } from '$lib/utils/errorMessage';
  import { CopyFeedback } from '$lib/dom/actionFeedback';
  const feedback = new CopyFeedback();

  /** Plain text to copy without activating a containing row or control. */
  let { value }: { value: string } = $props();

  async function copy(event: MouseEvent) {
    event.stopPropagation();
    const button = event.currentTarget as HTMLButtonElement;
    try {
      await navigator.clipboard.writeText(value);
      if (button.isConnected) feedback.show(button, m('common.copied_to_clipboard'));
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toastError(undefined, m('common.copy_failed'));
    }
  }
</script>

<button
  type="button"
  onclick={copy}
  class="copy-feedback mini-icon-action shrink-0"
  {@attach () => () => feedback.dispose()}
  aria-label={m('common.copy_to_clipboard')}
  title={m('common.copy_to_clipboard')}
>
  <span data-copy-icon aria-hidden="true" class="iconify icon-[uil--copy] text-base leading-none"
  ></span>
  <span
    data-copy-check
    aria-hidden="true"
    class="iconify icon-[uil--check] text-base leading-none text-action"
  ></span>
</button>
