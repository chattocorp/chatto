<script lang="ts">
  import { toast } from '$lib/ui/toast';

  let { value }: { value: string } = $props();

  async function copy(event: MouseEvent) {
    event.stopPropagation();
    await navigator.clipboard.writeText(value);
    toast.success('Copied to clipboard');
  }
</script>

<span class="inline-flex items-center gap-1.5">
  <code class="font-mono text-xs">{value}</code>
  <button type="button" onclick={copy} class="icon-action" title="Copy to clipboard">
    <span class="iconify text-sm uil--copy"></span>
  </button>
</span>
