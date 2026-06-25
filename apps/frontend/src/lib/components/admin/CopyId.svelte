<script lang="ts">
  import * as m from '$lib/i18n/messages';
  import { toast } from '$lib/ui/toast';

  let { value }: { value: string } = $props();

  async function copy(event: MouseEvent) {
    event.stopPropagation();
    await navigator.clipboard.writeText(value);
    toast.success(m['common.copied_to_clipboard']());
  }
</script>

<span class="inline-flex min-w-0 items-center gap-1.5 align-middle">
  <code class="font-mono text-xs leading-6">{value}</code>
  <button
    type="button"
    onclick={copy}
    class="icon-action shrink-0"
    title={m['common.copy_to_clipboard']()}
  >
    <span class="iconify text-base leading-none uil--copy"></span>
  </button>
</span>
