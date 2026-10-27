<script lang="ts">
  import { m } from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';
  import { toastError } from '$lib/utils/errorMessage';

  /** Plain text to copy without activating a containing row or control. */
  let { value }: { value: string } = $props();

  async function copy(event: MouseEvent) {
    event.stopPropagation();
    try {
      await navigator.clipboard.writeText(value);
      toast.success(m('common.copied_to_clipboard'));
    } catch {
      toastError(undefined, m('common.copy_failed'));
    }
  }
</script>

<button
  type="button"
  onclick={copy}
  class="mini-icon-action shrink-0"
  aria-label={m('common.copy_to_clipboard')}
  title={m('common.copy_to_clipboard')}
>
  <span aria-hidden="true" class="iconify icon-[uil--copy] text-base leading-none"></span>
</button>
