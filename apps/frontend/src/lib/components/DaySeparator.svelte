<script lang="ts">
  let {
    label,
    id = undefined,
    testId = undefined
  }: { label: string; id?: string; testId?: string } = $props();
</script>

<h2 {id} class="mt-4 flex w-full items-center gap-4 py-3" data-testid={testId}>
	<span class="h-px flex-1 bg-border"></span>
	<span class="text-xs font-medium text-muted">{label}</span>
	<span class="h-px flex-1 bg-border"></span>
</h2>
