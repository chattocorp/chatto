<script lang="ts">
  import { toast } from '$lib/ui/toast';

  let { value }: { value: string } = $props();

  async function copy(event: MouseEvent) {
    event.stopPropagation();
    await navigator.clipboard.writeText(value);
    toast.success('Copied to clipboard');
  }
</script>

<span class="inline-flex min-w-0 items-center gap-1.5 align-middle">
  <code class="font-mono text-xs leading-6">{value}</code>
  <button
    type="button"
    onclick={copy}
    class="hover:text-foreground inline-flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded text-muted hover:bg-surface-200"
    title="Copy to clipboard"
  >
    <span class="iconify uil--copy text-base leading-none"></span>
  </button>
</span>
